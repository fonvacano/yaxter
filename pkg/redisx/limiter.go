package redisx

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// slidingWindow atomically drops expired entries, checks the count, and
// records the request if under the limit. Returns 1 if allowed.
var slidingWindow = redis.NewScript(`
local key    = KEYS[1]
local now    = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit  = tonumber(ARGV[3])
local member = ARGV[4]
redis.call('ZREMRANGEBYSCORE', key, 0, now - window)
if redis.call('ZCARD', key) < limit then
  redis.call('ZADD', key, now, member)
  redis.call('PEXPIRE', key, window)
  return 1
end
return 0
`)

// Limiter is a Redis-backed sliding-window rate limiter.
// Keys are stored as rl:{key} per the ARCHITECTURE.md §2.3 key table.
type Limiter struct {
	rdb *redis.Client
	now func() time.Time
}

func NewLimiter(rdb *redis.Client) *Limiter {
	return &Limiter{rdb: rdb, now: time.Now}
}

// Allow reports whether one more request under key fits within limit per window.
func (l *Limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	res, err := slidingWindow.Run(ctx, l.rdb,
		[]string{"rl:" + key},
		l.now().UnixMilli(), window.Milliseconds(), limit, uuid.NewString(),
	).Int()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}
