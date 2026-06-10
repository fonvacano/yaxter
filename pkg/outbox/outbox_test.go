package outbox

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

func setup(t *testing.T) (context.Context, *tcpostgres.PostgresContainer, string) {
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
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	m, err := migrate.New("file://../../migrations", dsn)
	require.NoError(t, err)
	require.NoError(t, m.Up())
	_, _ = m.Close()
	return ctx, ctr, dsn
}

func TestInsertSharesTheCallersTransaction(t *testing.T) {
	ctx, _, dsn := setup(t)
	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	msg := Message{ID: 1001, Topic: "tweets.v1", Key: "2", Payload: []byte{0x1}}

	// Rollback: neither the domain row nor the outbox row survives.
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx,
		`INSERT INTO users (id, username, email) VALUES (2, 'alice', 'a@example.com')`)
	require.NoError(t, err)
	require.NoError(t, Insert(ctx, tx, msg))
	require.NoError(t, tx.Rollback(ctx))

	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM outbox`).Scan(&n))
	require.Zero(t, n)
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n))
	require.Zero(t, n)

	// Commit: both rows land atomically.
	tx, err = pool.Begin(ctx)
	require.NoError(t, err)
	_, err = tx.Exec(ctx,
		`INSERT INTO users (id, username, email) VALUES (2, 'alice', 'a@example.com')`)
	require.NoError(t, err)
	require.NoError(t, Insert(ctx, tx, msg))
	require.NoError(t, tx.Commit(ctx))

	var topic, key string
	var published *string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT topic, key, published_at::text FROM outbox WHERE id = 1001`,
	).Scan(&topic, &key, &published))
	require.Equal(t, "tweets.v1", topic)
	require.Equal(t, "2", key)
	require.Nil(t, published, "new rows are unpublished")
}

func TestInsertValidates(t *testing.T) {
	err := validate(Message{ID: 0, Topic: "t", Key: "k", Payload: []byte{1}})
	require.ErrorContains(t, err, "id")
	err = validate(Message{ID: 1, Topic: "", Key: "k", Payload: []byte{1}})
	require.ErrorContains(t, err, "topic")
	err = validate(Message{ID: 1, Topic: "t", Key: "", Payload: []byte{1}})
	require.ErrorContains(t, err, "key")
	require.NoError(t, validate(Message{ID: 1, Topic: "t", Key: "k", Payload: []byte{1}}))
}
