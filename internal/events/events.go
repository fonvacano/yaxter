// Package events provides thin helpers over pkg/outbox for domain event emission.
// It handles protobuf serialization, envelope construction, and traceparent extraction.
package events

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/fonvacano/yaxter/gen/yaxter/events/common/v1"
	"github.com/fonvacano/yaxter/pkg/outbox"
)

// hasEnvelope is satisfied by every top-level event message that carries
// an Envelope field (set by the caller before Emit).
type hasEnvelope interface {
	proto.Message
}

// NewEnvelope builds an Envelope with the given eventID, current time and
// the traceparent extracted from ctx (empty when no span is active).
func NewEnvelope(ctx context.Context, eventID int64) *commonv1.Envelope {
	return &commonv1.Envelope{
		EventId:     eventID,
		OccurredAt:  timestamppb.New(time.Now()),
		Traceparent: outbox.TraceparentFromContext(ctx),
		Producer:    "api",
	}
}

// Key returns the decimal string representation of id for use as the
// Kafka partition key (keeps related events on the same partition).
func Key(id int64) string { return fmt.Sprintf("%d", id) }

// Emit serialises msg, writes it to the outbox table inside tx, and returns
// any error. The caller must commit tx for the event to be published.
func Emit(ctx context.Context, tx pgx.Tx, eventID int64, topic string, key string, msg proto.Message) error {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("events: marshal: %w", err)
	}
	return outbox.Insert(ctx, tx, outbox.Message{
		ID:          eventID,
		Topic:       topic,
		Key:         key,
		Payload:     payload,
		Traceparent: outbox.TraceparentFromContext(ctx),
	})
}
