// Package redisx holds Redis helpers: client constructor, the sliding-window
// rate limiter (§7 of ARCHITECTURE.md), and the singleflight+LRU loader that
// fronts hot keys (§2.3).
package redisx

import "github.com/redis/go-redis/v9"

// NewClient returns a go-redis client for addr.
func NewClient(addr string) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: addr})
}
