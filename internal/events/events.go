// Package events provides helpers for transactional outbox emission (§2.4).
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

// NewEnvelope builds a common Envelope stamped with now.
func NewEnvelope(ctx context.Context, eventID int64) *commonv1.Envelope {
	return &commonv1.Envelope{
		EventId:     eventID,
		OccurredAt:  timestamppb.Now(),
		Traceparent: outbox.TraceparentFromContext(ctx),
		Producer:    "api",
	}
}

// Key returns a string partition key from an int64 (tweet id, user id, etc.).
func Key(id int64) string { return strconv.FormatInt(id, 10) }

// Emit serializes msg and inserts it into the outbox using tx.
func Emit(ctx context.Context, tx pgx.Tx, eventID int64, topic, key string, msg proto.Message) error {
	payload, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("events.Emit: marshal: %w", err)
	}
	return outbox.Insert(ctx, tx, outbox.Message{
		ID:          eventID,
		Topic:       topic,
		Key:         key,
		Payload:     payload,
		Traceparent: outbox.TraceparentFromContext(ctx),
	})
}
