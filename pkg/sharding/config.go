package sharding

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Physical struct {
	Name     string `yaml:"name"`
	DSN      string `yaml:"dsn"`
	Logicals string `yaml:"logicals"` // e.g. "0-255" or "0-63,128-191" or "5"
}

type Config struct {
	Physical []Physical `yaml:"physical"`
}

func ParseConfig(data []byte) (Config, error) {
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("sharding: parse config: %w", err)
	}
	return c, nil
}

// parseRanges expands "0-63,128-191" into the listed logical shard numbers.
func parseRanges(s string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		lo, hi := part, part
		if i := strings.IndexByte(part, '-'); i >= 0 {
			lo, hi = part[:i], part[i+1:]
		}
		l, err := strconv.Atoi(lo)
		if err != nil {
			return nil, fmt.Errorf("bad range %q: %w", part, err)
		}
		h, err := strconv.Atoi(hi)
		if err != nil {
			return nil, fmt.Errorf("bad range %q: %w", part, err)
		}
		if l > h || l < 0 || h >= NumLogicalShards {
			return nil, fmt.Errorf("range %q out of bounds [0,%d]", part, NumLogicalShards-1)
		}
		for n := l; n <= h; n++ {
			out = append(out, n)
		}
	}
	return out, nil
}
