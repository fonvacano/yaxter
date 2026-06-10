package gen_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/fonvacano/yaxter/gen/yaxter/events/common/v1"
	tweetsv1 "github.com/fonvacano/yaxter/gen/yaxter/events/tweets/v1"
)

func TestTweetEventRoundtrip(t *testing.T) {
	in := &tweetsv1.TweetEvent{
		Envelope: &commonv1.Envelope{
			EventId:     123456789,
			OccurredAt:  timestamppb.Now(),
			Traceparent: "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01",
			Producer:    "api@test",
		},
		Payload: &tweetsv1.TweetEvent_Created{Created: &tweetsv1.TweetCreated{
			TweetId:              1,
			AuthorId:             2,
			Text:                 "hello",
			MediaIds:             []int64{10, 11},
			AuthorFollowersCount: 42,
		}},
	}
	raw, err := proto.Marshal(in)
	require.NoError(t, err)

	var out tweetsv1.TweetEvent
	require.NoError(t, proto.Unmarshal(raw, &out))
	require.True(t, proto.Equal(in, &out))
	require.Equal(t, int64(123456789), out.GetEnvelope().GetEventId())
	require.Equal(t, "hello", out.GetCreated().GetText())
}
