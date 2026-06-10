package redisx

import (
	"context"
	"math/rand/v2"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"golang.org/x/sync/singleflight"
)

type entry[V any] struct {
	val V
	exp time.Time
}

// Loader is the in-process hot-key cache from ARCHITECTURE.md §2.3:
// a small LRU with jittered TTL (±20%, anti-stampede) fronted by
// singleflight so each pod runs at most one concurrent fetch per key.
type Loader[V any] struct {
	sf    singleflight.Group
	lru   *lru.Cache[string, entry[V]]
	ttl   time.Duration
	nowFn func() time.Time
	rnd   func() float64
}

func NewLoader[V any](size int, ttl time.Duration) (*Loader[V], error) {
	c, err := lru.New[string, entry[V]](size)
	if err != nil {
		return nil, err
	}
	return &Loader[V]{lru: c, ttl: ttl, nowFn: time.Now, rnd: rand.Float64}, nil
}

// Get returns the cached value for key, or runs fetch (deduplicated across
// goroutines) and caches the result with a jittered TTL. Errors are never cached.
func (l *Loader[V]) Get(ctx context.Context, key string, fetch func(context.Context) (V, error)) (V, error) {
	if e, ok := l.lru.Get(key); ok && l.nowFn().Before(e.exp) {
		return e.val, nil
	}
	v, err, _ := l.sf.Do(key, func() (any, error) {
		// Re-check: another flight may have filled the cache while we waited.
		if e, ok := l.lru.Get(key); ok && l.nowFn().Before(e.exp) {
			return e.val, nil
		}
		val, err := fetch(ctx)
		if err != nil {
			return nil, err
		}
		jittered := time.Duration(float64(l.ttl) * (0.8 + 0.4*l.rnd()))
		l.lru.Add(key, entry[V]{val: val, exp: l.nowFn().Add(jittered)})
		return val, nil
	})
	if err != nil {
		var zero V
		return zero, err
	}
	return v.(V), nil
}
