// Package kafkax constructs Kafka clients (ARCHITECTURE.md §2.4). Producers
// exist only inside the outbox relay (T1.0) — api never talks to Kafka.
// Consumer-group IDs are stable contracts: "yaxter.<role>", identical in
// demo and production.
package kafkax

import (
	"errors"

	"github.com/twmb/franz-go/pkg/kgo"
)

// NewClient builds a franz-go client. Connection is lazy; readiness is the
// caller's concern (worker readiness probes, T3.2).
func NewClient(brokers []string, opts ...kgo.Opt) (*kgo.Client, error) {
	if len(brokers) == 0 {
		return nil, errors.New("kafkax: at least one broker required")
	}
	return kgo.NewClient(append([]kgo.Opt{kgo.SeedBrokers(brokers...)}, opts...)...)
}

// GroupID returns the canonical consumer-group id for a worker role.
func GroupID(role string) string { return "yaxter." + role }
