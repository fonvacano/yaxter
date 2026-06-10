package relay

import (
	"context"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/fonvacano/yaxter/pkg/outbox"
)

// KafkaPublisher publishes batches with franz-go. ProduceSync with a single
// client preserves per-partition order (same key => same partition), which is
// the per-aggregate ordering contract from docs/events.md.
type KafkaPublisher struct {
	client *kgo.Client
}

func NewKafkaPublisher(client *kgo.Client) *KafkaPublisher {
	return &KafkaPublisher{client: client}
}

func (p *KafkaPublisher) Publish(ctx context.Context, msgs []outbox.Message) error {
	records := make([]*kgo.Record, len(msgs))
	for i, m := range msgs {
		rec := &kgo.Record{
			Topic: m.Topic,
			Key:   []byte(m.Key),
			Value: m.Payload,
		}
		if m.Traceparent != "" {
			rec.Headers = append(rec.Headers, kgo.RecordHeader{
				Key: "traceparent", Value: []byte(m.Traceparent),
			})
		}
		records[i] = rec
	}
	return p.client.ProduceSync(ctx, records...).FirstErr()
}
