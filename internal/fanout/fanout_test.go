package fanout

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/fonvacano/yaxter/gen/yaxter/events/common/v1"
	tweetsv1 "github.com/fonvacano/yaxter/gen/yaxter/events/tweets/v1"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
)

// newRedis spins an in-process miniredis and returns a connected client.
func newRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr := miniredis.RunT(t)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func createdEvent(eventID, tweetID, authorID int64, followers int32) *tweetsv1.TweetEvent {
	return &tweetsv1.TweetEvent{
		Envelope: &commonv1.Envelope{EventId: eventID, OccurredAt: timestamppb.New(time.Unix(0, 0)), Producer: "api"},
		Payload: &tweetsv1.TweetEvent_Created{Created: &tweetsv1.TweetCreated{
			TweetId: tweetID, AuthorId: authorID, AuthorFollowersCount: followers,
		}},
	}
}

func TestSubThresholdAuthorFansOutToAllFollowers(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	pool := newPoolWithFollowers(t, ctx, 7, []int64{10, 11, 12}) // author 7 followed by 10,11,12
	rdb := newRedis(t)
	f := New(pool, rdb, 50, NewMetrics(prometheus.NewRegistry()))

	require.NoError(t, f.HandleEvent(ctx, createdEvent(900, 555, 7, 3)))

	for _, follower := range []int64{10, 11, 12} {
		got, err := rdb.LRange(ctx, tlKey(follower), 0, -1).Result()
		require.NoError(t, err)
		require.Equal(t, []string{"555"}, got)
	}
}

func TestCelebrityAuthorIsSkipped(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	pool := newPoolWithFollowers(t, ctx, 8, []int64{20, 21})
	rdb := newRedis(t)
	f := New(pool, rdb, 50, NewMetrics(prometheus.NewRegistry()))

	require.NoError(t, f.HandleEvent(ctx, createdEvent(901, 556, 8, 50))) // followers == threshold → celebrity

	for _, follower := range []int64{20, 21} {
		n, err := rdb.Exists(ctx, tlKey(follower)).Result()
		require.NoError(t, err)
		require.EqualValues(t, 0, n, "celebrity tweets must not be fanned out")
	}
}

func TestRedeliveryDoesNotDoublePush(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	pool := newPoolWithFollowers(t, ctx, 9, []int64{30})
	rdb := newRedis(t)
	f := New(pool, rdb, 50, NewMetrics(prometheus.NewRegistry()))

	ev := createdEvent(902, 557, 9, 1)
	require.NoError(t, f.HandleEvent(ctx, ev))
	require.NoError(t, f.HandleEvent(ctx, ev)) // redelivery

	got, err := rdb.LRange(ctx, tlKey(30), 0, -1).Result()
	require.NoError(t, err)
	require.Equal(t, []string{"557"}, got, "event_id dedupe must collapse redelivery")
}

// newPoolWithFollowers boots a PG testcontainer, runs migrations, and inserts
// rows into the `followers` reverse-edge table so the author has the given
// follower ids. Returns a live pool.
func newPoolWithFollowers(t *testing.T, ctx context.Context, authorID int64, followers []int64) *pgxpool.Pool {
	t.Helper()
	pool := migratedPool(t, ctx)
	for _, fid := range followers {
		_, err := pool.Exec(ctx,
			`INSERT INTO followers (followee_id, follower_id) VALUES ($1, $2)`, authorID, fid)
		require.NoError(t, err)
	}
	return pool
}

func migratedPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
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
	_, _ = m.Close()
	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func deletedEvent(eventID, tweetID, authorID int64) *tweetsv1.TweetEvent {
	return &tweetsv1.TweetEvent{
		Envelope: &commonv1.Envelope{EventId: eventID, OccurredAt: timestamppb.New(time.Unix(0, 0)), Producer: "api"},
		Payload:  &tweetsv1.TweetEvent_Deleted{Deleted: &tweetsv1.TweetDeleted{TweetId: tweetID, AuthorId: authorID}},
	}
}

func TestDeleteRemovesIdFromSubThresholdFollowerLists(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	pool := newPoolWithFollowers(t, ctx, 40, []int64{50, 51})
	// author 40 is sub-threshold: set followers_count low.
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, followers_count, following_count, created_at)
		 VALUES (40, 'a40', 'a40@x.io', 2, 0, now())`)
	require.NoError(t, err)
	rdb := newRedis(t)
	f := New(pool, rdb, 50, NewMetrics(prometheus.NewRegistry()))

	// pre-seed both follower timelines with the tweet id (and a survivor)
	for _, follower := range []int64{50, 51} {
		require.NoError(t, rdb.RPush(ctx, tlKey(follower), 700, 701).Err())
	}
	require.NoError(t, f.HandleEvent(ctx, deletedEvent(910, 700, 40)))

	for _, follower := range []int64{50, 51} {
		got, err := rdb.LRange(ctx, tlKey(follower), 0, -1).Result()
		require.NoError(t, err)
		require.Equal(t, []string{"701"}, got, "deleted id removed, survivor kept")
	}
}
