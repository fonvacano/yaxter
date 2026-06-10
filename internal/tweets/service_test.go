package tweets

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/protobuf/proto"

	tweetsv1 "github.com/fonvacano/yaxter/gen/yaxter/events/tweets/v1"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

func testService(t *testing.T) (*Service, *pgxpool.Pool, *miniredis.Miniredis) {
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

	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, username, email, pass_hash, followers_count) VALUES
		(1, 'author', 'a@example.com', 'x', 7),
		(2, 'other',  'o@example.com', 'x', 0)`)
	require.NoError(t, err)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	gen, err := snowflake.New(5)
	require.NoError(t, err)
	return NewService(pool, rdb, gen), pool, mr
}

func lastTweetEvent(t *testing.T, pool *pgxpool.Pool) *tweetsv1.TweetEvent {
	t.Helper()
	var payload []byte
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT payload FROM outbox WHERE topic = 'tweets.v1'
		ORDER BY id DESC LIMIT 1`).Scan(&payload))
	var ev tweetsv1.TweetEvent
	require.NoError(t, proto.Unmarshal(payload, &ev))
	return &ev
}

func TestCreateTweetPersistsEmitsAndCaches(t *testing.T) {
	svc, pool, mr := testService(t)
	ctx := context.Background()

	tw, err := svc.Create(ctx, 1, "hello world", nil, 0)
	require.NoError(t, err)
	require.NotZero(t, tw.ID)

	var text string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT text FROM tweets WHERE id = $1`, tw.ID).Scan(&text))
	require.Equal(t, "hello world", text)

	ev := lastTweetEvent(t, pool)
	require.Equal(t, tw.ID, ev.GetCreated().GetTweetId())
	require.EqualValues(t, 7, ev.GetCreated().GetAuthorFollowersCount(),
		"event snapshots the author's follower count for the fan-out threshold")

	utl, err := mr.List("utl:1")
	require.NoError(t, err)
	require.Len(t, utl, 1, "author's own timeline cache must be appended (§2.1)")
	require.True(t, mr.Exists(tweetKey(tw.ID)), "tweet body cached")
}

func TestCreateValidation(t *testing.T) {
	svc, pool, _ := testService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, 1, "", nil, 0)
	require.ErrorIs(t, err, ErrValidation, "empty non-retweet rejected")

	long := make([]rune, 281)
	for i := range long {
		long[i] = 'x'
	}
	_, err = svc.Create(ctx, 1, string(long), nil, 0)
	require.ErrorIs(t, err, ErrValidation)

	_, err = pool.Exec(ctx, `
		INSERT INTO media (id, owner_id, content_type, size_bytes, status) VALUES
		(601, 1, 'image/webp', 9, 'ready'), (602, 1, 'image/webp', 9, 'pending')`)
	require.NoError(t, err)
	_, err = svc.Create(ctx, 1, "with media", []int64{601, 602}, 0)
	require.ErrorIs(t, err, ErrMediaNotReady)
	_, err = svc.Create(ctx, 1, "with media", []int64{601}, 0)
	require.NoError(t, err)
}

func TestRetweetFlattensAndEmitsEngagement(t *testing.T) {
	svc, pool, _ := testService(t)
	ctx := context.Background()

	orig, err := svc.Create(ctx, 1, "original", nil, 0)
	require.NoError(t, err)
	rt1, err := svc.Create(ctx, 2, "", nil, orig.ID)
	require.NoError(t, err)
	require.Equal(t, orig.ID, rt1.RetweetOfID)

	rt2, err := svc.Create(ctx, 1, "", nil, rt1.ID)
	require.NoError(t, err)
	require.Equal(t, orig.ID, rt2.RetweetOfID)

	var n int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT count(*) FROM outbox WHERE topic = 'engagements.v1'`).Scan(&n))
	require.Equal(t, 2, n, "each retweet emits one engagement event for counters")

	_, err = svc.Create(ctx, 1, "", nil, 999999)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestDeleteOwnTweetOnly(t *testing.T) {
	svc, pool, mr := testService(t)
	ctx := context.Background()

	tw, err := svc.Create(ctx, 1, "to delete", nil, 0)
	require.NoError(t, err)

	require.ErrorIs(t, svc.Delete(ctx, 2, tw.ID), ErrForbidden)
	require.NoError(t, svc.Delete(ctx, 1, tw.ID))
	require.ErrorIs(t, svc.Delete(ctx, 1, tw.ID), ErrNotFound)

	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM tweets`).Scan(&n))
	require.Zero(t, n)
	ev := lastTweetEvent(t, pool)
	require.Equal(t, tw.ID, ev.GetDeleted().GetTweetId())
	require.False(t, mr.Exists(tweetKey(tw.ID)), "tw: cache dropped on delete")
	utl, _ := mr.List("utl:1")
	require.Empty(t, utl, "utl: entry removed on delete")
}

func TestGetHydratesAuthorAndCounters(t *testing.T) {
	svc, pool, _ := testService(t)
	ctx := context.Background()

	tw, err := svc.Create(ctx, 1, "readable", nil, 0)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`UPDATE tweets SET likes_count = 5 WHERE id = $1`, tw.ID)
	require.NoError(t, err)

	got, err := svc.Get(ctx, tw.ID)
	require.NoError(t, err)
	require.Equal(t, "readable", got.Text)
	require.Equal(t, "author", got.AuthorUsername)
	require.Equal(t, 5, got.LikesCount)

	_, err = svc.Get(ctx, 424242)
	require.ErrorIs(t, err, ErrNotFound)
}
