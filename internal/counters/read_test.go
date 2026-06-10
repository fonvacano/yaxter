package counters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadThroughPrefersRedisFallsBackToPG(t *testing.T) {
	c, pool, rdb := testCounters(t)
	ctx := context.Background()

	_, err := pool.Exec(ctx,
		`UPDATE tweets SET likes_count = 3, retweets_count = 1 WHERE id = 10`)
	require.NoError(t, err)
	likes, retweets, err := Read(ctx, rdb, pool, 10)
	require.NoError(t, err)
	require.Equal(t, 3, likes)
	require.Equal(t, 1, retweets)

	require.NoError(t, rdb.HSet(ctx, "cnt:10", "likes", 5, "retweets", 1).Err())
	likes, _, err = Read(ctx, rdb, pool, 10)
	require.NoError(t, err)
	require.Equal(t, 5, likes)
	_ = c
}

func TestReconcileCorrectsDrift(t *testing.T) {
	c, pool, rdb := testCounters(t)
	ctx := context.Background()

	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email) VALUES (2,'b','b@x.c'), (3,'c','c@x.c')`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO likes (user_id, tweet_id) VALUES (2, 10), (3, 10)`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`UPDATE tweets SET likes_count = 7 WHERE id = 10`)
	require.NoError(t, err)
	require.NoError(t, rdb.HSet(ctx, "cnt:10", "likes", 7).Err())

	require.NoError(t, Reconcile(ctx, pool, rdb))

	var likes int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT likes_count FROM tweets WHERE id = 10`).Scan(&likes))
	require.Equal(t, 2, likes, "reconcile recomputes from the likes table")
	hot, err := rdb.HGet(ctx, "cnt:10", "likes").Int()
	require.NoError(t, err)
	require.Equal(t, 2, hot, "hot hash corrected too")
	_ = c
}
