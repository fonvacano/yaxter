package users

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFollowersPagination(t *testing.T) {
	svc, pool, _ := testService(t)
	ctx := context.Background()
	seedUser(t, pool, 100, "star")
	for i := int64(1); i <= 5; i++ {
		seedUser(t, pool, i, "fan"+string(rune('a'+i-1)))
		require.NoError(t, svc.Follow(ctx, i, "star"))
	}

	page1, next, err := svc.Followers(ctx, "star", 0, 2)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	require.NotZero(t, next)

	page2, next2, err := svc.Followers(ctx, "star", next, 2)
	require.NoError(t, err)
	require.Len(t, page2, 2)

	page3, next3, err := svc.Followers(ctx, "star", next2, 2)
	require.NoError(t, err)
	require.Len(t, page3, 1)
	require.Zero(t, next3, "last page has no cursor")

	seen := map[int64]bool{}
	for _, u := range append(append(page1, page2...), page3...) {
		require.False(t, seen[u.ID], "no duplicates across pages")
		seen[u.ID] = true
	}
	require.Len(t, seen, 5)

	following, _, err := svc.Following(ctx, "fana", 0, 10)
	require.NoError(t, err)
	require.Len(t, following, 1)
	require.Equal(t, "star", following[0].Username)
}
