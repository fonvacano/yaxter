package kafkax

import (
	"context"
	"errors"

	"github.com/rs/zerolog"
	"github.com/twmb/franz-go/pkg/kgo"
)

// Handler processes a single Kafka record.
type Handler func(ctx context.Context, rec *kgo.Record) error

// Consume polls the client in a tight loop, calling h for each record.
// It commits offsets only after h succeeds. Returns nil on ctx cancellation.
func Consume(ctx context.Context, client *kgo.Client, log zerolog.Logger, h Handler) error {
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		fetches := client.PollRecords(ctx, 100)
		if errs := fetches.Errors(); len(errs) > 0 {
			for _, fe := range errs {
				if errors.Is(fe.Err, context.Canceled) || errors.Is(fe.Err, context.DeadlineExceeded) {
					return nil
				}
				log.Error().Err(fe.Err).Str("topic", fe.Topic).Int32("partition", fe.Partition).Msg("fetch error")
			}
			continue
		}
		fetches.EachRecord(func(rec *kgo.Record) {
			if err := h(ctx, rec); err != nil {
				log.Error().Err(err).Str("topic", rec.Topic).Int64("offset", rec.Offset).Msg("handler error")
			}
		})
		if err := client.CommitUncommittedOffsets(ctx); err != nil && ctx.Err() == nil {
			log.Warn().Err(err).Msg("commit offsets failed")
		}
	}
}
