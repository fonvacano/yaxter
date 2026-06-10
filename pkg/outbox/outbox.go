// Package outbox implements the producer half of the transactional outbox
// (ARCHITECTURE.md §2.4): events are inserted in the SAME transaction as the
// domain write — atomicity is the database's. The relay worker (T1.0)
// publishes and deletes the rows; api never talks to Kafka.
package outbox

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/trace"
)

// Message is one event bound for Kafka. ID is a snowflake and defines
// publish order; Key becomes the Kafka record key (partition affinity);
// Traceparent (optional) is copied to the Kafka `traceparent` header by
// the relay so one trace spans write -> fan-out -> read.
type Message struct {
	ID          int64
	Topic       string
	Key         string
	Payload     []byte // serialized protobuf event wrapper
	Traceparent string
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
		`INSERT INTO outbox (id, topic, key, payload, traceparent)
		 VALUES ($1, $2, $3, $4, NULLIF($5, ''))`,
		msg.ID, msg.Topic, msg.Key, msg.Payload, msg.Traceparent)
	return err
}

// TraceparentFromContext renders the active span context as a W3C
// traceparent header value, or "" when no valid span is present.
func TraceparentFromContext(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}
	return fmt.Sprintf("00-%s-%s-%s", sc.TraceID(), sc.SpanID(), sc.TraceFlags())
}
