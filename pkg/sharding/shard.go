// Package sharding implements the 256-logical-shard routing from
// ARCHITECTURE.md §2.2. NumLogicalShards and LogicalShard are permanent
// contracts: production remaps logical shards to new physical clusters by
// editing the shard map — never by changing the hash.
package sharding

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
)

const NumLogicalShards = 256

// LogicalShard maps a shard key (user_id, author_id, ...) to its logical
// shard: FNV-1a(64) over the key's big-endian bytes, mod 256.
func LogicalShard(key int64) int {
	h := fnv.New64a()
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(key))
	_, _ = h.Write(b[:])
	return int(h.Sum64() % NumLogicalShards)
}

// Map resolves logical shards to physical clusters.
type Map struct {
	physicals []Physical
	byLogical [NumLogicalShards]int // logical -> index into physicals
}

func NewMap(cfg Config) (*Map, error) {
	m := &Map{physicals: cfg.Physical}
	for i := range m.byLogical {
		m.byLogical[i] = -1
	}
	for pi, p := range cfg.Physical {
		logicals, err := parseRanges(p.Logicals)
		if err != nil {
			return nil, fmt.Errorf("sharding: physical %q: %w", p.Name, err)
		}
		for _, l := range logicals {
			if m.byLogical[l] != -1 {
				return nil, fmt.Errorf("sharding: logical shard %d assigned twice", l)
			}
			m.byLogical[l] = pi
		}
	}
	for l, pi := range m.byLogical {
		if pi == -1 {
			return nil, fmt.Errorf("sharding: logical shard %d unassigned", l)
		}
	}
	return m, nil
}

// PhysicalFor returns the physical cluster owning key's logical shard.
func (m *Map) PhysicalFor(key int64) Physical {
	return m.physicals[m.byLogical[LogicalShard(key)]]
}

// Physicals returns all configured physical clusters.
func (m *Map) Physicals() []Physical { return m.physicals }
