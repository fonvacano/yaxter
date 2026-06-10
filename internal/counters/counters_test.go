package counters

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	commonv1 "github.com/fonvacano/yaxter/gen/yaxter/events/common/v1"
	engagementsv1 "github.com/fonvacano/yaxter/gen/yaxter/events/engagements/v1"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
)

func testCounters(t *testing.T) (*Counters, *pgxpool.Pool, *redis.Client) {
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
	_, err = pool.Exec(ctx,
		`INSERT INTO users (id, username, email) VALUES (1, 'a', 'a@x.c')`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO tweets (id, author_id, text) VALUES (10, 1, 'hot')`)
	require.NoError(t, err)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return New(pool, rdb, 500, 2*time.Second), pool, rdb
}

func likedEvent(eventID int64, tweetID int64) *engagementsv1.EngagementEvent {
	return &engagementsv1.EngagementEvent{
		Envelope: &commonv1.Envelope{EventId: eventID},
		Payload: &engagementsv1.EngagementEvent_Liked{Liked: &engagementsv1.TweetLiked{
			TweetId: tweetID, UserId: 2, AuthorId: 1,
		}},
	}
}

func TestThousandLikesBatchedExactlyOnce(t *testing.T) {
	c, pool, rdb := testCounters(t)
	ctx := context.Background()

	for i := int64(1); i <= 1000; i++ {
		require.NoError(t, c.HandleEvent(ctx, likedEvent(i, 10)))
	}
	for i := int64(1); i <= 200; i++ {
		require.NoError(t, c.HandleEvent(ctx, likedEvent(i, 10))) // replays
	}
	require.NoError(t, c.Flush(ctx))

	var likes int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT likes_count FROM tweets WHERE id = 10`).Scan(&likes))
	require.Equal(t, 1000, likes, "exact count despite replays")

	flushes := c.FlushCount()
	require.LessOrEqual(t, flushes, 3, "1000 events / 500 per flush (+ final)")
	require.GreaterOrEqual(t, flushes, 2)

	hot, err := rdb.HGet(ctx, "cnt:10", "likes").Int()
	require.NoError(t, err)
	require.Equal(t, 1000, hot, "redis hash tracks the live value (§2.7)")
}

func TestUnlikeAndRetweetDeltas(t *testing.T) {
	c, pool, _ := testCounters(t)
	ctx := context.Background()

	require.NoError(t, c.HandleEvent(ctx, likedEvent(1, 10)))
	require.NoError(t, c.HandleEvent(ctx, &engagementsv1.EngagementEvent{
		Envelope: &commonv1.Envelope{EventId: 2},
		Payload: &engagementsv1.EngagementEvent_Unliked{Unliked: &engagementsv1.TweetUnliked{
			TweetId: 10, UserId: 2, AuthorId: 1,
		}},
	}))
	require.NoError(t, c.HandleEvent(ctx, &engagementsv1.EngagementEvent{
		Envelope: &commonv1.Envelope{EventId: 3},
		Payload: &engagementsv1.EngagementEvent_Retweeted{Retweeted: &engagementsv1.TweetRetweeted{
			TweetId: 10, RetweetId: 11, UserId: 2, AuthorId: 1,
		}},
	}))
	require.NoError(t, c.Flush(ctx))

	var likes, retweets int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT likes_count, retweets_count FROM tweets WHERE id = 10`).
		Scan(&likes, &retweets))
	require.Zero(t, likes)
	require.Equal(t, 1, retweets)
}
