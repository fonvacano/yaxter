package snowflake

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
)

func setupPG(t *testing.T) *tcpostgres.PostgresContainer {
	t.Helper()
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
	return ctr
}

func TestLeaseAssignsDistinctNodeIDs(t *testing.T) {
	ctr := setupPG(t)
	ctx := context.Background()
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	a, err := AcquireLease(ctx, pool, "pod-a", time.Minute)
	require.NoError(t, err)
	b, err := AcquireLease(ctx, pool, "pod-b", time.Minute)
	require.NoError(t, err)

	require.NotEqual(t, a.NodeID(), b.NodeID())
	require.GreaterOrEqual(t, a.NodeID(), int64(0))
	require.LessOrEqual(t, b.NodeID(), int64(MaxNode))
}

func TestExpiredLeaseIsReacquirable(t *testing.T) {
	ctr := setupPG(t)
	ctx := context.Background()
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	// 1ms TTL: the lease is expired by the time the second acquire runs.
	a, err := AcquireLease(ctx, pool, "pod-a", time.Millisecond)
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	b, err := AcquireLease(ctx, pool, "pod-b", time.Minute)
	require.NoError(t, err)
	require.Equal(t, a.NodeID(), b.NodeID(), "expired node id must be reused first")
}

func TestHeartbeatExtendsLease(t *testing.T) {
	ctr := setupPG(t)
	ctx := context.Background()
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	a, err := AcquireLease(ctx, pool, "pod-a", 200*time.Millisecond)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		time.Sleep(100 * time.Millisecond)
		require.NoError(t, a.Heartbeat(ctx))
	}
	// Despite >200ms elapsed, the heartbeats kept the lease alive:
	// a fresh acquire must get a different node id.
	b, err := AcquireLease(ctx, pool, "pod-b", time.Minute)
	require.NoError(t, err)
	require.NotEqual(t, a.NodeID(), b.NodeID())
}
