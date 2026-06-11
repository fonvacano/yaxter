package notifications

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	engagementsv1 "github.com/fonvacano/yaxter/gen/yaxter/events/engagements/v1"
	followsv1 "github.com/fonvacano/yaxter/gen/yaxter/events/follows/v1"
	"github.com/fonvacano/yaxter/pkg/redisx"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

type Worker struct {
	db  *pgxpool.Pool
	rdb *redis.Client
	ids *snowflake.Generator
}

func NewWorker(db *pgxpool.Pool, rdb *redis.Client, ids *snowflake.Generator) *Worker {
	return &Worker{db: db, rdb: rdb, ids: ids}
}

// HandleRecord dispatches by topic (the consumer subscribes to both topics).
func (w *Worker) HandleRecord(ctx context.Context, rec *kgo.Record) error {
	switch rec.Topic {
	case "follows.v1":
		return w.HandleFollow(ctx, rec)
	case "engagements.v1":
		return w.HandleEngagement(ctx, rec)
	default:
		return nil
	}
}

func (w *Worker) HandleFollow(ctx context.Context, rec *kgo.Record) error {
	var ev followsv1.FollowEvent
	if err := proto.Unmarshal(rec.Value, &ev); err != nil {
		return fmt.Errorf("notifications: unmarshal follow: %w", err)
	}
	first, err := w.once(ctx, ev.GetEnvelope().GetEventId())
	if err != nil || !first {
		return err
	}
	return w.handleFollow(ctx, &ev)
}

func (w *Worker) HandleEngagement(ctx context.Context, rec *kgo.Record) error {
	var ev engagementsv1.EngagementEvent
	if err := proto.Unmarshal(rec.Value, &ev); err != nil {
		return fmt.Errorf("notifications: unmarshal engagement: %w", err)
	}
	first, err := w.once(ctx, ev.GetEnvelope().GetEventId())
	if err != nil || !first {
		return err
	}
	return w.handleEngagement(ctx, &ev)
}

func (w *Worker) once(ctx context.Context, eventID int64) (bool, error) {
	return redisx.Once(ctx, w.rdb, fmt.Sprintf("evt:notif:%d", eventID), 24*time.Hour)
}

// handleFollow / handleEngagement are the dedupe-free cores (test entry points).
func (w *Worker) handleFollow(ctx context.Context, ev *followsv1.FollowEvent) error {
	fc := ev.GetFollowChanged()
	if fc == nil || !fc.GetFollowing() {
		return nil // unfollow → no notification
	}
	return insert(ctx, w.db, w.ids.Next(), fc.GetFolloweeId(), KindFollow, fc.GetFollowerId(), nil)
}

func (w *Worker) handleEngagement(ctx context.Context, ev *engagementsv1.EngagementEvent) error {
	switch p := ev.GetPayload().(type) {
	case *engagementsv1.EngagementEvent_Liked:
		return w.notifyEngagement(ctx, KindLike, p.Liked.GetAuthorId(), p.Liked.GetUserId(), p.Liked.GetTweetId())
	case *engagementsv1.EngagementEvent_Retweeted:
		return w.notifyEngagement(ctx, KindRetweet, p.Retweeted.GetAuthorId(), p.Retweeted.GetUserId(), p.Retweeted.GetTweetId())
	default:
		return nil // unlike / unretweet → no notification
	}
}

func (w *Worker) notifyEngagement(ctx context.Context, kind string, authorID, actorID, tweetID int64) error {
	if authorID == actorID {
		return nil // self-action suppression
	}
	subject := tweetID
	return insert(ctx, w.db, w.ids.Next(), authorID, kind, actorID, &subject)
}
