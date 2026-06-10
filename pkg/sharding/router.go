package sharding

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ForEachShard groups keys by owning physical shard (preserving input order
// within a group) and calls fn once per physical. Cross-shard reads batch
// per physical cluster (§2.2); demo cardinality makes this one call.
func (m *Map) ForEachShard(keys []int64, fn func(p Physical, keys []int64) error) error {
	groups := make(map[int][]int64)
	var order []int
	for _, k := range keys {
		pi := m.byLogical[LogicalShard(k)]
		if _, ok := groups[pi]; !ok {
			order = append(order, pi)
		}
		groups[pi] = append(groups[pi], k)
	}
	for _, pi := range order {
		if err := fn(m.physicals[pi], groups[pi]); err != nil {
			return err
		}
	}
	return nil
}

// Router owns one pgx pool per physical cluster.
type Router struct {
	m     *Map
	pools map[string]*pgxpool.Pool
}

func NewRouter(ctx context.Context, m *Map) (*Router, error) {
	r := &Router{m: m, pools: make(map[string]*pgxpool.Pool, len(m.physicals))}
	for _, p := range m.physicals {
		pool, err := pgxpool.New(ctx, p.DSN)
		if err != nil {
			r.Close()
			return nil, fmt.Errorf("sharding: dial %q: %w", p.Name, err)
		}
		if err := pool.Ping(ctx); err != nil {
			pool.Close()
			r.Close()
			return nil, fmt.Errorf("sharding: ping %q: %w", p.Name, err)
		}
		r.pools[p.Name] = pool
	}
	return r, nil
}

// Pool returns the pool owning key's shard.
func (r *Router) Pool(key int64) *pgxpool.Pool {
	return r.pools[r.m.PhysicalFor(key).Name]
}

// ForEachShard runs fn once per physical shard holding any of keys.
func (r *Router) ForEachShard(keys []int64, fn func(pool *pgxpool.Pool, keys []int64) error) error {
	return r.m.ForEachShard(keys, func(p Physical, ks []int64) error {
		return fn(r.pools[p.Name], ks)
	})
}

func (r *Router) Close() {
	for _, p := range r.pools {
		p.Close()
	}
}
