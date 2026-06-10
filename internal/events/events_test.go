package events

import (
	"context"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/protobuf/proto"

	tweetsv1 "github.com/fonvacano/yaxter/gen/yaxter/events/tweets/v1"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
)

func TestKey(t *testing.T) {
	require.Equal(t, "12345", Key(12345))
}

func TestNewEnvelopeStampsFields(t *testing.T) {
	env := NewEnvelope(context.Background(), 777)
	require.EqualValues(t, 777, env.GetEventId())
	require.NotNil(t, env.GetOccurredAt())
	require.Equal(t, "api", env.GetProducer())
}

func TestEmitWritesOutboxRowInCallersTx(t *testing.T) {
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

	ev := &tweetsv1.TweetEvent{
		Envelope: NewEnvelope(ctx, 901),
		Payload: &tweetsv1.TweetEvent_Created{Created: &tweetsv1.TweetCreated{
			TweetId: 1, AuthorId: 2, Text: "hi",
		}},
	}
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, Emit(ctx, tx, 901, "tweets.v1", Key(2), ev))
	require.NoError(t, tx.Commit(ctx))

	var payload []byte
	var topic, key string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT topic, key, payload FROM outbox WHERE id = 901`).
		Scan(&topic, &key, &payload))
	require.Equal(t, "tweets.v1", topic)
	require.Equal(t, "2", key)

	var out tweetsv1.TweetEvent
	require.NoError(t, proto.Unmarshal(payload, &out))
	require.Equal(t, "hi", out.GetCreated().GetText())
}
