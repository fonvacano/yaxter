package redisx

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Once returns true the first time key is seen, false on replay.
// It uses SET NX with the given TTL for at-most-once deduplication.
func Once(ctx context.Context, rdb *redis.Client, key string, ttl time.Duration) (bool, error) {
	ok, err := rdb.SetNX(ctx, key, 1, ttl).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}
