package snowflake

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewRejectsInvalidNode(t *testing.T) {
	_, err := New(-1)
	require.Error(t, err)
	_, err = New(1024)
	require.Error(t, err)
}

func TestNextUniqueAndMonotonic(t *testing.T) {
	g, err := New(7)
	require.NoError(t, err)

	const n = 10000
	seen := make(map[int64]struct{}, n)
	prev := int64(0)
	for i := 0; i < n; i++ {
		id := g.Next()
		require.Greater(t, id, prev, "ids must strictly increase")
		_, dup := seen[id]
		require.False(t, dup)
		seen[id] = struct{}{}
		prev = id
	}
}

func TestNextEmbedsNodeAndTimestamp(t *testing.T) {
	g, err := New(513)
	require.NoError(t, err)
	before := time.Now()
	id := g.Next()

	require.EqualValues(t, 513, Node(id))
	ts := Timestamp(id)
	require.WithinDuration(t, before, ts, time.Second)
}

func TestClockRegressionDoesNotGoBackward(t *testing.T) {
	g, _ := New(1)
	now := time.Now()
	calls := 0
	g.clock = func() time.Time {
		calls++
		if calls == 2 {
			return now.Add(-time.Hour) // clock jumps back
		}
		return now.Add(time.Duration(calls) * time.Millisecond)
	}
	first := g.Next()
	second := g.Next() // generated against the regressed clock
	require.Greater(t, second, first)
}
