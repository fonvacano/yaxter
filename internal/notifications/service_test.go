package notifications

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListUnreadAndMarkRead(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	pool := migratedPool(t, ctx)
	// actor user row (List joins users for actor display fields)
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, followers_count, following_count, created_at)
		 VALUES (2, 'bob', 'bob@x.io', 0, 0, now())`)
	require.NoError(t, err)
	// three notifications for user 1 from actor 2
	for _, nid := range []int64{10, 11, 12} {
		require.NoError(t, insert(ctx, pool, nid, 1, KindFollow, 2, nil))
	}
	svc := NewService(pool)

	// unread count = 3
	cnt, err := svc.UnreadCount(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 3, cnt)

	// list newest-first with actor hydration
	items, next, err := svc.List(ctx, 1, 0, 2)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.EqualValues(t, 12, items[0].ID)
	require.Equal(t, "bob", items[0].ActorName)
	require.NotNil(t, next)
	require.EqualValues(t, 11, *next)

	// mark read up to id 11 → only id 12 remains unread
	require.NoError(t, svc.MarkRead(ctx, 1, 11))
	cnt, err = svc.UnreadCount(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 1, cnt)
}
