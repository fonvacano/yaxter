package redisx

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newTestLimiter(t *testing.T) (*Limiter, *time.Time) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	now := time.Now()
	l := NewLimiter(rdb)
	l.now = func() time.Time { return now }
	return l, &now
}

func TestAllowUpToLimitThenDeny(t *testing.T) {
	l, _ := newTestLimiter(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		ok, err := l.Allow(ctx, "u1:tweets", 3, time.Minute)
		require.NoError(t, err)
		require.True(t, ok, "request %d should be allowed", i+1)
	}
	ok, err := l.Allow(ctx, "u1:tweets", 3, time.Minute)
	require.NoError(t, err)
	require.False(t, ok, "4th request must be denied")
}

func TestWindowSlides(t *testing.T) {
	l, now := newTestLimiter(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		ok, _ := l.Allow(ctx, "u1:tweets", 3, time.Minute)
		require.True(t, ok)
	}
	*now = now.Add(61 * time.Second) // old entries fall out of the window
	ok, err := l.Allow(ctx, "u1:tweets", 3, time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestKeysAreIndependent(t *testing.T) {
	l, _ := newTestLimiter(t)
	ctx := context.Background()

	ok, _ := l.Allow(ctx, "u1:tweets", 1, time.Minute)
	require.True(t, ok)
	ok, _ = l.Allow(ctx, "u2:tweets", 1, time.Minute)
	require.True(t, ok, "different subject must have its own budget")
}
