package kafkax

import (
	"context"

	"github.com/rs/zerolog"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Handler processes one record.
type Handler func(ctx context.Context, rec *kgo.Record) error

// Consume polls until ctx ends, passing every record to h. A handler error
// is logged and the record skipped — delivery is at-least-once with
// consumer-side dedupe (docs/events.md rule 1), and a poison record must
// not wedge its partition. Offsets use kgo's group autocommit.
func Consume(ctx context.Context, client *kgo.Client, log zerolog.Logger, h Handler) error {
	for {
		fetches := client.PollFetches(ctx)
		if err := ctx.Err(); err != nil {
			return err
		}
		if fetches.IsClientClosed() {
			return nil
		}
		fetches.EachError(func(topic string, partition int32, err error) {
			log.Error().Err(err).Str("topic", topic).Int32("partition", partition).
				Msg("fetch error")
		})
		fetches.EachRecord(func(rec *kgo.Record) {
			if err := h(ctx, rec); err != nil {
				log.Error().Err(err).Str("topic", rec.Topic).
					Int64("offset", rec.Offset).Msg("handler error; record skipped")
			}
		})
	}
}
