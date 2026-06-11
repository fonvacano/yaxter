package timeline

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/fonvacano/yaxter/gen/yaxter/events/common/v1"
	tweetsv1 "github.com/fonvacano/yaxter/gen/yaxter/events/tweets/v1"
	"github.com/fonvacano/yaxter/internal/fanout"
)

func TestFanoutThenTimelineReadEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	pool := migratedPool(t, ctx)
	seedUser(t, ctx, pool, 1, "reader", 0)
	seedUser(t, ctx, pool, 2, "followee", 3) // sub-threshold
	seedFollow(t, ctx, pool, 1, 2)
	seedTweet(t, ctx, pool, 12000, 2, "real tweet")
	svc, rdb := newService(t, pool)

	// drive the actual fan-out worker (no Kafka — call HandleEvent directly)
	f := fanout.New(pool, rdb, 50, fanout.NewMetrics(prometheus.NewRegistry()))
	ev := &tweetsv1.TweetEvent{
		Envelope: &commonv1.Envelope{EventId: 1, OccurredAt: timestamppb.Now(), Producer: "api"},
		Payload: &tweetsv1.TweetEvent_Created{Created: &tweetsv1.TweetCreated{
			TweetId: 12000, AuthorId: 2, AuthorFollowersCount: 3}},
	}
	require.NoError(t, f.HandleEvent(ctx, ev))

	// reader's warm-cache read sees the fanned-out tweet
	warm, _, err := svc.Home(ctx, 1, 0, 20)
	require.NoError(t, err)
	require.Len(t, warm, 1)
	require.EqualValues(t, 12000, warm[0].ID)

	// cold-cache rebuild equals the warm result (DoD: rebuild == warm)
	require.NoError(t, rdb.Del(ctx, "tl:1").Err())
	cold, _, err := svc.Home(ctx, 1, 0, 20)
	require.NoError(t, err)
	require.Len(t, cold, 1)
	require.EqualValues(t, warm[0].ID, cold[0].ID, "cold rebuild matches warm read")
}
