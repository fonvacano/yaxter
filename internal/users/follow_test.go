package users

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFollowWritesBothEdgesCountsAndOneEvent(t *testing.T) {
	svc, pool, _ := testService(t)
	ctx := context.Background()
	seedUser(t, pool, 1, "alice")
	seedUser(t, pool, 2, "bob")

	require.NoError(t, svc.Follow(ctx, 1, "bob"))
	// Idempotent double-follow: no second edge, no second event.
	require.NoError(t, svc.Follow(ctx, 1, "bob"))

	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM follows WHERE follower_id=1 AND followee_id=2`).Scan(&n))
	require.Equal(t, 1, n)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM followers WHERE followee_id=2 AND follower_id=1`).Scan(&n))
	require.Equal(t, 1, n, "reverse edge must exist (§2.2)")

	require.NoError(t, pool.QueryRow(ctx,
		`SELECT followers_count FROM users WHERE id=2`).Scan(&n))
	require.Equal(t, 1, n)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT following_count FROM users WHERE id=1`).Scan(&n))
	require.Equal(t, 1, n)

	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE topic='follows.v1'`).Scan(&n))
	require.Equal(t, 1, n, "exactly one event per state change")
}

func TestUnfollowRemovesEdgesAndEmitsOnce(t *testing.T) {
	svc, pool, _ := testService(t)
	ctx := context.Background()
	seedUser(t, pool, 1, "alice")
	seedUser(t, pool, 2, "bob")
	require.NoError(t, svc.Follow(ctx, 1, "bob"))

	require.NoError(t, svc.Unfollow(ctx, 1, "bob"))
	require.NoError(t, svc.Unfollow(ctx, 1, "bob")) // idempotent

	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM follows`).Scan(&n))
	require.Zero(t, n)
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM followers`).Scan(&n))
	require.Zero(t, n)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT followers_count FROM users WHERE id=2`).Scan(&n))
	require.Zero(t, n)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE topic='follows.v1'`).Scan(&n))
	require.Equal(t, 2, n, "one follow event + one unfollow event")
}

func TestFollowGuards(t *testing.T) {
	svc, pool, _ := testService(t)
	ctx := context.Background()
	seedUser(t, pool, 1, "alice")

	require.ErrorIs(t, svc.Follow(ctx, 1, "alice"), ErrSelfFollow)
	require.ErrorIs(t, svc.Follow(ctx, 1, "ghost"), ErrNotFound)
}

func TestFollowingACelebrityInvalidatesCelebsCache(t *testing.T) {
	svc, pool, mr := testService(t) // threshold = 50
	ctx := context.Background()
	seedUser(t, pool, 1, "alice")
	seedUser(t, pool, 2, "celeb")
	_, err := pool.Exec(ctx, `UPDATE users SET followers_count = 60 WHERE id = 2`)
	require.NoError(t, err)
	_ = mr.Set(fmt.Sprintf("celebs:%d", 1), "stale")

	require.NoError(t, svc.Follow(ctx, 1, "celeb"))
	require.False(t, mr.Exists("celebs:1"),
		"celebs set must be invalidated on follow of a celebrity (§2.3)")
}
