package redisx

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestOnceIsFirstWriterWins(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ctx := context.Background()

	first, err := Once(ctx, rdb, "evt:1", time.Hour)
	require.NoError(t, err)
	require.True(t, first)

	again, err := Once(ctx, rdb, "evt:1", time.Hour)
	require.NoError(t, err)
	require.False(t, again, "replays must be detected")

	other, err := Once(ctx, rdb, "evt:2", time.Hour)
	require.NoError(t, err)
	require.True(t, other)
}
