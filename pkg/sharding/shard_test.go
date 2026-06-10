package sharding

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogicalShardDeterministicAndInRange(t *testing.T) {
	for _, key := range []int64{0, 1, 42, 1<<40 + 12345, -7} {
		s1 := LogicalShard(key)
		s2 := LogicalShard(key)
		require.Equal(t, s1, s2)
		require.GreaterOrEqual(t, s1, 0)
		require.Less(t, s1, NumLogicalShards)
	}
}

// Snowflake-shaped keys (low 12 bits = sequence, usually 0) must still
// spread across shards — this is the test that forbids `key % 256`.
func TestLogicalShardDistributionOnSnowflakes(t *testing.T) {
	shards := make(map[int]bool)
	for i := int64(0); i < 10000; i++ {
		key := (1700000000000+i)<<22 | 5<<12 // seq always 0, node always 5
		shards[LogicalShard(key)] = true
	}
	require.GreaterOrEqual(t, len(shards), 200,
		"10k snowflake keys must hit most of the 256 shards")
}

func TestNewMapValidatesCoverage(t *testing.T) {
	// gap
	_, err := NewMap(Config{Physical: []Physical{
		{Name: "a", DSN: "x", Logicals: "0-100"},
	}})
	require.ErrorContains(t, err, "unassigned")

	// overlap
	_, err = NewMap(Config{Physical: []Physical{
		{Name: "a", DSN: "x", Logicals: "0-200"},
		{Name: "b", DSN: "y", Logicals: "100-255"},
	}})
	require.ErrorContains(t, err, "twice")

	// exact cover, split ranges
	m, err := NewMap(Config{Physical: []Physical{
		{Name: "a", DSN: "x", Logicals: "0-63,128-191"},
		{Name: "b", DSN: "y", Logicals: "64-127,192-255"},
	}})
	require.NoError(t, err)
	require.NotNil(t, m)
}

func TestPhysicalForConsistentWithLogicalShard(t *testing.T) {
	m, err := NewMap(Config{Physical: []Physical{
		{Name: "a", DSN: "x", Logicals: "0-127"},
		{Name: "b", DSN: "y", Logicals: "128-255"},
	}})
	require.NoError(t, err)

	for _, key := range []int64{1, 999, 123456789} {
		want := "a"
		if LogicalShard(key) >= 128 {
			want = "b"
		}
		require.Equal(t, want, m.PhysicalFor(key).Name)
	}
}

func TestParseConfigYAML(t *testing.T) {
	cfg, err := ParseConfig([]byte(`
physical:
  - name: demo
    dsn: postgres://yaxter:yaxter@localhost:5432/yaxter
    logicals: "0-255"
`))
	require.NoError(t, err)
	m, err := NewMap(cfg)
	require.NoError(t, err)
	require.Equal(t, "demo", m.PhysicalFor(42).Name)
}
