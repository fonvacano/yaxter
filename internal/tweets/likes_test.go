package tweets

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLikeIsIdempotentAndEmitsOncePerChange(t *testing.T) {
	svc, pool, _ := testService(t)
	ctx := context.Background()
	tw, err := svc.Create(ctx, 1, "likeable", nil, 0)
	require.NoError(t, err)

	require.NoError(t, svc.Like(ctx, 2, tw.ID))
	require.NoError(t, svc.Like(ctx, 2, tw.ID)) // idempotent

	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM likes`).Scan(&n))
	require.Equal(t, 1, n)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE topic='engagements.v1'`).Scan(&n))
	require.Equal(t, 1, n, "double-like emits exactly one event")

	require.NoError(t, svc.Unlike(ctx, 2, tw.ID))
	require.NoError(t, svc.Unlike(ctx, 2, tw.ID)) // idempotent
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM likes`).Scan(&n))
	require.Zero(t, n)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox WHERE topic='engagements.v1'`).Scan(&n))
	require.Equal(t, 2, n)

	require.ErrorIs(t, svc.Like(ctx, 2, 99999), ErrNotFound)
}
