// Package events stamps and emits domain events through the transactional
// outbox (§2.4). Every producer in every module goes through Emit — there is
// no other path to the broker.
package events

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/fonvacano/yaxter/gen/yaxter/events/common/v1"
	"github.com/fonvacano/yaxter/pkg/outbox"
)

// Key renders an int64 aggregate id as the Kafka record key
// (decimal string per docs/events.md).
func Key(id int64) string { return strconv.FormatInt(id, 10) }

// NewEnvelope stamps an event with its id, time, and trace context.
// Consumers dedupe on EventId (docs/events.md rule 1).
func NewEnvelope(ctx context.Context, eventID int64) *commonv1.Envelope {
	return &commonv1.Envelope{
		EventId:     eventID,
		OccurredAt:  timestamppb.Now(),
		Traceparent: outbox.TraceparentFromContext(ctx),
		Producer:    "api",
	}
}

// Emit marshals the topic wrapper message and inserts it into the outbox in
// the caller's transaction — the same tx as the domain write.
func Emit(ctx context.Context, tx pgx.Tx, eventID int64, topic, key string, msg proto.Message) error {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("events: marshal %s: %w", topic, err)
	}
	return outbox.Insert(ctx, tx, outbox.Message{
		ID:          eventID,
		Topic:       topic,
		Key:         key,
		Payload:     payload,
		Traceparent: outbox.TraceparentFromContext(ctx),
	})
}
