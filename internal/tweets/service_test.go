package tweets

import (
	"context"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/redisx"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

func testService(t *testing.T) (*Service, *pgxpool.Pool, *redis.Client) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("yaxter"), tcpostgres.WithUsername("yaxter"),
		tcpostgres.WithPassword("yaxter"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	m, err := migrate.New("file://../../migrations", dsn)
	require.NoError(t, err)
	require.NoError(t, m.Up())
	m.Close()
	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, seedUser(ctx, pool, 1, "alice"))

	rdb := redisx.NewClient("localhost:6379") // miniredis not needed here; redis ops are best-effort
	ids, err := snowflake.New(1)
	require.NoError(t, err)
	return NewService(pool, rdb, ids), pool, rdb
}

func seedUser(ctx context.Context, pool *pgxpool.Pool, id int64, username string) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		id, username, username+"@example.com")
	return err
}
