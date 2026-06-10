package relay

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/fonvacano/yaxter/pkg/outbox"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
)

// fakePublisher records publishes in order and can be told to fail.
type fakePublisher struct {
	mu     sync.Mutex
	got    []outbox.Message
	failTo error // when non-nil, Publish fails
}

func (f *fakePublisher) Publish(_ context.Context, msgs []outbox.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failTo != nil {
		return f.failTo
	}
	f.got = append(f.got, msgs...)
	return nil
}

func (f *fakePublisher) published() []outbox.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]outbox.Message(nil), f.got...)
}

func (f *fakePublisher) setFail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failTo = err
}

func migratedPool(t *testing.T) *pgxpool.Pool {
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
	m.Close()
	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func insertOutboxRows(t *testing.T, pool *pgxpool.Pool, ids ...int64) {
	t.Helper()
	ctx := context.Background()
	for _, id := range ids {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		require.NoError(t, outbox.Insert(ctx, tx, outbox.Message{
			ID: id, Topic: "tweets.v1", Key: "7", Payload: []byte{byte(id)},
		}))
		require.NoError(t, tx.Commit(ctx))
	}
}

func newTestRelay(pool *pgxpool.Pool, pub Publisher) *Relay {
	cfg := DefaultConfig()
	cfg.PollInterval = 10 * time.Millisecond
	return New(pool, pub, cfg, NewMetrics(nil), zerolog.Nop())
}

func TestPublishesInOrderAndMarks(t *testing.T) {
	pool := migratedPool(t)
	ctx := context.Background()
	insertOutboxRows(t, pool, 3, 1, 2) // inserted out of order on purpose

	pub := &fakePublisher{}
	r := newTestRelay(pool, pub)
	require.NoError(t, r.cycle(ctx))

	got := pub.published()
	require.Len(t, got, 3)
	require.Equal(t, []int64{1, 2, 3},
		[]int64{got[0].ID, got[1].ID, got[2].ID},
		"publish order must be snowflake order")

	var unpublished int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE published_at IS NULL`).Scan(&unpublished))
	require.Zero(t, unpublished)
}

func TestPublisherFailureLeavesRowsUnpublished(t *testing.T) {
	pool := migratedPool(t)
	ctx := context.Background()
	insertOutboxRows(t, pool, 10, 11)

	pub := &fakePublisher{}
	pub.setFail(errors.New("kafka down"))
	r := newTestRelay(pool, pub)
	require.Error(t, r.cycle(ctx))

	var unpublished int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE published_at IS NULL`).Scan(&unpublished))
	require.Equal(t, 2, unpublished, "no row may be marked when publish failed")

	// Recovery: next cycle drains everything that accumulated meanwhile.
	insertOutboxRows(t, pool, 12)
	pub.setFail(nil)
	require.NoError(t, r.cycle(ctx))
	require.Len(t, pub.published(), 3)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE published_at IS NULL`).Scan(&unpublished))
	require.Zero(t, unpublished)
}

func TestCycleDrainsBacklogLargerThanBatch(t *testing.T) {
	pool := migratedPool(t)
	ctx := context.Background()
	ids := make([]int64, 0, 7)
	for i := int64(100); i < 107; i++ {
		ids = append(ids, i)
	}
	insertOutboxRows(t, pool, ids...)

	pub := &fakePublisher{}
	cfg := DefaultConfig()
	cfg.BatchSize = 3 // force multiple inner batches
	r := New(pool, pub, cfg, NewMetrics(nil), zerolog.Nop())
	require.NoError(t, r.cycle(ctx))
	require.Len(t, pub.published(), 7, "one cycle drains the whole backlog")
}
