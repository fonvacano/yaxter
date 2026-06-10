// Package outbox implements the producer half of the transactional outbox
// (ARCHITECTURE.md §2.4): events are inserted in the SAME transaction as the
// domain write — atomicity is the database's. The relay worker (T1.0)
// publishes and deletes the rows; api never talks to Kafka.
package outbox

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Message is one event bound for Kafka. ID is a snowflake and defines
// publish order; Key becomes the Kafka record key (partition affinity).
type Message struct {
	ID      int64
	Topic   string
	Key     string
	Payload []byte // serialized protobuf event wrapper
}

func validate(m Message) error {
	switch {
	case m.ID == 0:
		return errors.New("outbox: message id required")
	case m.Topic == "":
		return errors.New("outbox: topic required")
	case m.Key == "":
		return errors.New("outbox: key required")
	}
	return nil
}

// Insert writes msg into the outbox using the caller's transaction.
// The caller MUST pass the same tx that performs the domain write.
func Insert(ctx context.Context, tx pgx.Tx, msg Message) error {
	if err := validate(msg); err != nil {
		return err
	}
	_, err := tx.Exec(ctx,
		`INSERT INTO outbox (id, topic, key, payload) VALUES ($1, $2, $3, $4)`,
		msg.ID, msg.Topic, msg.Key, msg.Payload)
	return err
}
