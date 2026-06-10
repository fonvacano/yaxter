// Package redisx holds Redis helpers: client constructor, the sliding-window
// rate limiter (§7 of ARCHITECTURE.md), and the singleflight+LRU loader that
// fronts hot keys (§2.3).
package redisx

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewClient returns a go-redis client for addr.
func NewClient(addr string) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: addr})
}

// Once reports whether key is being seen for the first time within ttl
// (SETNX) — the consumer-side event_id dedupe primitive (§2.7).
func Once(ctx context.Context, rdb *redis.Client, key string, ttl time.Duration) (bool, error) {
	return rdb.SetNX(ctx, key, "1", ttl).Result()
}
