package sharding

import (
	"context"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// ForEachShard is pure Map logic — unit-testable with a 2-entry fake map
// (the T0.4 DoD case). In the demo the grouping degenerates to one batch,
// but the code path is exercised here with two.
func TestForEachShardGroupsKeys(t *testing.T) {
	m, err := NewMap(Config{Physical: []Physical{
		{Name: "a", DSN: "x", Logicals: "0-127"},
		{Name: "b", DSN: "y", Logicals: "128-255"},
	}})
	require.NoError(t, err)

	keys := make([]int64, 0, 1000)
	for i := int64(1); i <= 1000; i++ {
		keys = append(keys, i*7919) // arbitrary spread
	}

	var got []int64
	seen := map[string][]int64{}
	err = m.ForEachShard(keys, func(p Physical, shardKeys []int64) error {
		seen[p.Name] = append(seen[p.Name], shardKeys...)
		got = append(got, shardKeys...)
		for _, k := range shardKeys {
			want := "a"
			if LogicalShard(k) >= 128 {
				want = "b"
			}
			require.Equal(t, want, p.Name, "key %d routed to wrong shard", k)
		}
		return nil
	})
	require.NoError(t, err)
	require.Len(t, seen, 2, "1000 spread keys must hit both physicals")

	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	require.Equal(t, keys, got, "every key visited exactly once")
}

func TestRouterPoolsAgainstRealPG(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("yaxter"),
		tcpostgres.WithUsername("yaxter"),
		tcpostgres.WithPassword("yaxter"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	m, err := NewMap(Config{Physical: []Physical{
		{Name: "demo", DSN: dsn, Logicals: "0-255"},
	}})
	require.NoError(t, err)

	r, err := NewRouter(ctx, m)
	require.NoError(t, err)
	t.Cleanup(r.Close)

	var one int
	require.NoError(t, r.Pool(42).QueryRow(ctx, "SELECT 1").Scan(&one))
	require.Equal(t, 1, one)

	calls := 0
	err = r.ForEachShard([]int64{1, 2, 3}, func(pool *pgxpool.Pool, keys []int64) error {
		calls++
		require.Len(t, keys, 3)
		return pool.QueryRow(ctx, "SELECT 1").Scan(&one)
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls, "single physical => single batch")

	var g int
	require.NoError(t, r.GlobalPool().QueryRow(ctx, "SELECT 2").Scan(&g))
	require.Equal(t, 2, g)
}
