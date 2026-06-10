package redisx

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoaderCachesValue(t *testing.T) {
	l, err := NewLoader[string](16, time.Minute)
	require.NoError(t, err)
	ctx := context.Background()

	var calls atomic.Int32
	fetch := func(context.Context) (string, error) {
		calls.Add(1)
		return "v", nil
	}
	for i := 0; i < 5; i++ {
		v, err := l.Get(ctx, "k", fetch)
		require.NoError(t, err)
		require.Equal(t, "v", v)
	}
	require.EqualValues(t, 1, calls.Load())
}

func TestLoaderSingleflight(t *testing.T) {
	l, err := NewLoader[string](16, time.Minute)
	require.NoError(t, err)
	ctx := context.Background()

	var calls atomic.Int32
	fetch := func(context.Context) (string, error) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		return "v", nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := l.Get(ctx, "hot", fetch)
			require.NoError(t, err)
			require.Equal(t, "v", v)
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, calls.Load(), "concurrent gets must share one fetch")
}

func TestLoaderExpiryWithJitterBounds(t *testing.T) {
	l, err := NewLoader[string](16, 10*time.Second)
	require.NoError(t, err)
	ctx := context.Background()

	now := time.Now()
	l.nowFn = func() time.Time { return now }

	_, err = l.Get(ctx, "k", func(context.Context) (string, error) { return "v1", nil })
	require.NoError(t, err)

	// Jitter is ±20%: entry must still be live at 8s-ε and dead by 12s+ε.
	var calls atomic.Int32
	refetch := func(context.Context) (string, error) {
		calls.Add(1)
		return "v2", nil
	}
	now = now.Add(7900 * time.Millisecond)
	v, _ := l.Get(ctx, "k", refetch)
	require.Equal(t, "v1", v)
	require.EqualValues(t, 0, calls.Load())

	now = now.Add(4200 * time.Millisecond) // total 12.1s > max jittered TTL
	v, _ = l.Get(ctx, "k", refetch)
	require.Equal(t, "v2", v)
	require.EqualValues(t, 1, calls.Load())
}

func TestLoaderFetchErrorNotCached(t *testing.T) {
	l, err := NewLoader[string](16, time.Minute)
	require.NoError(t, err)
	ctx := context.Background()

	boom := errors.New("boom")
	_, err = l.Get(ctx, "k", func(context.Context) (string, error) { return "", boom })
	require.ErrorIs(t, err, boom)

	v, err := l.Get(ctx, "k", func(context.Context) (string, error) { return "ok", nil })
	require.NoError(t, err)
	require.Equal(t, "ok", v)
}
