package timeline

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

	"github.com/fonvacano/yaxter/internal/tweets"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

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

func seedUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id int64, name string, followers int) {
	t.Helper()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, followers_count, following_count, created_at)
		 VALUES ($1, $2, $3, $4, 0, now())`, id, name, name+"@x.io", followers)
	require.NoError(t, err)
}

func seedFollow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, follower, followee int64) {
	t.Helper()
	_, err := pool.Exec(ctx, `INSERT INTO follows (follower_id, followee_id) VALUES ($1, $2)`, follower, followee)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO followers (followee_id, follower_id) VALUES ($1, $2)`, followee, follower)
	require.NoError(t, err)
}

func seedTweet(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id, author int64, text string) {
	t.Helper()
	_, err := pool.Exec(ctx,
		`INSERT INTO tweets (id, author_id, text, media, created_at, likes_count, retweets_count)
		 VALUES ($1, $2, $3, '[]'::jsonb, now(), 0, 0)`, id, author, text)
	require.NoError(t, err)
}

func newService(t *testing.T, pool *pgxpool.Pool) (*Service, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	gen, err := snowflake.New(6)
	require.NoError(t, err)
	tweetsSvc := tweets.NewService(pool, rdb, gen)
	svc, err := NewService(pool, rdb, tweetsSvc, 50)
	require.NoError(t, err)
	return svc, rdb
}

func TestHomeReadsFanoutList(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	pool := migratedPool(t, ctx)
	seedUser(t, ctx, pool, 1, "reader", 0)
	seedUser(t, ctx, pool, 2, "followee", 3)
	seedFollow(t, ctx, pool, 1, 2)
	seedTweet(t, ctx, pool, 5000, 2, "hi from followee")
	svc, rdb := newService(t, pool)
	// simulate fan-out having pushed the tweet id into the reader's list
	require.NoError(t, rdb.LPush(ctx, "tl:1", 5000).Err())

	items, next, err := svc.Home(ctx, 1, 0, 20)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.EqualValues(t, 5000, items[0].ID)
	require.Equal(t, "followee", items[0].AuthorUsername)
	require.Nil(t, next)
}

func TestHomeRebuildsOnCacheMiss(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	pool := migratedPool(t, ctx)
	seedUser(t, ctx, pool, 1, "reader", 0)
	seedUser(t, ctx, pool, 2, "followee", 3) // sub-threshold author
	seedFollow(t, ctx, pool, 1, 2)
	seedTweet(t, ctx, pool, 6000, 2, "older")
	seedTweet(t, ctx, pool, 6001, 2, "newer")
	svc, rdb := newService(t, pool)
	// tl:1 deliberately empty → rebuild path

	items, _, err := svc.Home(ctx, 1, 0, 20)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.EqualValues(t, 6001, items[0].ID, "newest first")
	require.EqualValues(t, 6000, items[1].ID)
	// rebuild must have warmed tl:1
	cached, err := rdb.LRange(ctx, "tl:1", 0, -1).Result()
	require.NoError(t, err)
	require.Equal(t, []string{"6001", "6000"}, cached)
}

func TestHomeMergesCelebrityStream(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	pool := migratedPool(t, ctx)
	seedUser(t, ctx, pool, 1, "reader", 0)
	seedUser(t, ctx, pool, 2, "normal", 3)  // sub-threshold
	seedUser(t, ctx, pool, 3, "celeb", 100) // over-threshold (>=50)
	seedFollow(t, ctx, pool, 1, 2)
	seedFollow(t, ctx, pool, 1, 3)
	seedTweet(t, ctx, pool, 7000, 2, "from normal")
	seedTweet(t, ctx, pool, 7002, 3, "from celeb (newer id)")
	svc, rdb := newService(t, pool)
	require.NoError(t, rdb.LPush(ctx, "tl:1", 7000).Err())  // fan-out delivered the normal tweet
	require.NoError(t, rdb.LPush(ctx, "utl:3", 7002).Err()) // celeb's own profile stream

	items, _, err := svc.Home(ctx, 1, 0, 20)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.EqualValues(t, 7002, items[0].ID, "celeb merged by snowflake id, newer first")
	require.EqualValues(t, 7000, items[1].ID)
}

func TestHomeCursorPagination(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	pool := migratedPool(t, ctx)
	seedUser(t, ctx, pool, 1, "reader", 0)
	seedUser(t, ctx, pool, 2, "followee", 3)
	seedFollow(t, ctx, pool, 1, 2)
	ids := []int64{8000, 8001, 8002}
	for _, id := range ids {
		seedTweet(t, ctx, pool, id, 2, "t")
	}
	svc, rdb := newService(t, pool)
	require.NoError(t, rdb.RPush(ctx, "tl:1", 8002, 8001, 8000).Err()) // newest-first

	page1, next, err := svc.Home(ctx, 1, 0, 2)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.EqualValues(t, 8002, page1[0].ID)
	require.EqualValues(t, 8001, page1[1].ID)
	require.NotNil(t, next)
	require.EqualValues(t, 8001, *next)

	page2, next2, err := svc.Home(ctx, 1, *next, 2)
	require.NoError(t, err)
	require.Len(t, page2, 1)
	require.EqualValues(t, 8000, page2[0].ID)
	require.Nil(t, next2)
}

func TestProfileTimelineByUsername(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	pool := migratedPool(t, ctx)
	seedUser(t, ctx, pool, 2, "author", 3)
	seedTweet(t, ctx, pool, 9000, 2, "first")
	seedTweet(t, ctx, pool, 9001, 2, "second")
	svc, _ := newService(t, pool)

	items, next, err := svc.Profile(ctx, "author", 0, 20)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.EqualValues(t, 9001, items[0].ID, "newest-first")
	require.Nil(t, next)
}

func TestProfileTimelineUnknownUser(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	pool := migratedPool(t, ctx)
	svc, _ := newService(t, pool)
	_, _, err := svc.Profile(ctx, "ghost", 0, 20)
	require.ErrorIs(t, err, ErrUserNotFound)
}
