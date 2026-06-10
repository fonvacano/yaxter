// Package snowflake generates time-ordered 64-bit IDs (ARCHITECTURE.md §2.6):
// 41 bits of milliseconds since 2025-01-01, 10 bits node, 12 bits sequence.
// Time-ordering makes timeline merges a plain int64 k-way merge and cursor
// pagination index-only.
package snowflake

import (
	"fmt"
	"sync"
	"time"
)

const (
	// EpochMs is 2025-01-01T00:00:00Z in Unix milliseconds.
	EpochMs  = int64(1735689600000)
	nodeBits = 10
	seqBits  = 12
	MaxNode  = (1 << nodeBits) - 1 // 1023
	maxSeq   = (1 << seqBits) - 1  // 4095
)

type Generator struct {
	mu     sync.Mutex
	node   int64
	lastMs int64
	seq    int64
	clock  func() time.Time
}

func New(node int64) (*Generator, error) {
	if node < 0 || node > MaxNode {
		return nil, fmt.Errorf("snowflake: node %d out of range [0,%d]", node, MaxNode)
	}
	return &Generator{node: node, clock: time.Now}, nil
}

// Next returns the next ID. Safe for concurrent use. If the wall clock
// regresses, generation continues from the last observed millisecond so IDs
// never go backward.
func (g *Generator) Next() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.clock().UnixMilli()
	if now < g.lastMs {
		now = g.lastMs
	}
	if now == g.lastMs {
		g.seq = (g.seq + 1) & maxSeq
		if g.seq == 0 { // sequence exhausted within this ms: wait for the next one
			for now <= g.lastMs {
				now = g.clock().UnixMilli()
				if now < g.lastMs {
					now = g.lastMs + 1
				}
			}
		}
	} else {
		g.seq = 0
	}
	g.lastMs = now
	return (now-EpochMs)<<(nodeBits+seqBits) | g.node<<seqBits | g.seq
}

// Node extracts the node ID embedded in id.
func Node(id int64) int64 { return (id >> seqBits) & MaxNode }

// Timestamp extracts the creation time embedded in id.
func Timestamp(id int64) time.Time {
	return time.UnixMilli((id >> (nodeBits + seqBits)) + EpochMs)
}
