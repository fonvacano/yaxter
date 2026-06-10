package migrations_test

import (
	"context"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
)

var allTables = []string{
	"users", "identities", "global_identities",
	"follows", "followers",
	"tweets", "likes",
	"notifications",
	"refresh_tokens", "idempotency",
	"outbox", "node_ids",
}

func TestMigrationsUpDownUp(t *testing.T) {
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

	m, err := migrate.New("file://.", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = m.Close() })

	require.NoError(t, m.Up())

	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	for _, tbl := range allTables {
		var reg *string
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT to_regclass($1)::text`, tbl).Scan(&reg))
		require.NotNil(t, reg, "table %s must exist after up", tbl)
	}

	require.NoError(t, m.Down())
	var reg *string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT to_regclass('users')::text`).Scan(&reg))
	require.Nil(t, reg, "users must be gone after down")

	require.NoError(t, m.Up(), "re-up after down must be clean")
}
