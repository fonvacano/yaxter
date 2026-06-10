package relay

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/fonvacano/yaxter/pkg/kafkax"
	"github.com/fonvacano/yaxter/pkg/outbox"
)

func kafkaBrokers(t *testing.T) []string {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	ctr, err := tckafka.Run(ctx, "confluentinc/confluent-local:7.7.0",
		tckafka.WithClusterID("yaxter-test"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	brokers, err := ctr.Brokers(ctx)
	require.NoError(t, err)
	return brokers
}

func TestKafkaPublisherDeliversInOrderWithHeaders(t *testing.T) {
	brokers := kafkaBrokers(t)
	ctx := context.Background()

	client, err := kafkax.NewClient(brokers, kgo.AllowAutoTopicCreation())
	require.NoError(t, err)
	t.Cleanup(client.Close)
	pub := NewKafkaPublisher(client)

	tp := "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"
	msgs := []outbox.Message{
		{ID: 1, Topic: "tweets.v1", Key: "7", Payload: []byte("a"), Traceparent: tp},
		{ID: 2, Topic: "tweets.v1", Key: "7", Payload: []byte("b")},
		{ID: 3, Topic: "tweets.v1", Key: "7", Payload: []byte("c")},
	}
	require.NoError(t, pub.Publish(ctx, msgs))
	// At-least-once duplicate: re-publishing the same batch must succeed;
	// consumers dedupe on envelope.event_id (docs/events.md rule 1).
	require.NoError(t, pub.Publish(ctx, msgs[:1]))

	consumer, err := kafkax.NewClient(brokers,
		kgo.ConsumeTopics("tweets.v1"),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	require.NoError(t, err)
	t.Cleanup(consumer.Close)

	var values []string
	var firstHeaders map[string]string
	deadline := time.After(30 * time.Second)
	for len(values) < 4 {
		select {
		case <-deadline:
			t.Fatalf("timed out; got %v", values)
		default:
		}
		fetches := consumer.PollFetches(ctx)
		require.NoError(t, fetches.Err())
		fetches.EachRecord(func(rec *kgo.Record) {
			values = append(values, string(rec.Value))
			if firstHeaders == nil {
				firstHeaders = map[string]string{}
				for _, h := range rec.Headers {
					firstHeaders[h.Key] = string(h.Value)
				}
			}
		})
	}
	// Same key => same partition => order preserved; the duplicate lands after.
	require.Equal(t, []string{"a", "b", "c", "a"}, values)
	require.Equal(t, tp, firstHeaders["traceparent"])
}
