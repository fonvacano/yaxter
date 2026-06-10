package tweets

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	engagementsv1 "github.com/fonvacano/yaxter/gen/yaxter/events/engagements/v1"
	"github.com/fonvacano/yaxter/internal/events"
)

// Like records the like (idempotent via PK) and emits TweetLiked in the same tx.
func (s *Service) Like(ctx context.Context, userID, tweetID int64) error {
	return s.setLike(ctx, userID, tweetID, true)
}

// Unlike removes the like (idempotent) and emits TweetUnliked in the same tx.
func (s *Service) Unlike(ctx context.Context, userID, tweetID int64) error {
	return s.setLike(ctx, userID, tweetID, false)
}

func (s *Service) setLike(ctx context.Context, userID, tweetID int64, like bool) error {
	var authorID int64
	err := s.db.QueryRow(ctx,
		`SELECT author_id FROM tweets WHERE id = $1`, tweetID).Scan(&authorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var changed bool
	if like {
		tag, err := tx.Exec(ctx, `
			INSERT INTO likes (user_id, tweet_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, userID, tweetID)
		if err != nil {
			return err
		}
		changed = tag.RowsAffected() == 1
	} else {
		tag, err := tx.Exec(ctx,
			`DELETE FROM likes WHERE user_id = $1 AND tweet_id = $2`, userID, tweetID)
		if err != nil {
			return err
		}
		changed = tag.RowsAffected() == 1
	}
	if !changed {
		return nil
	}

	eventID := s.ids.Next()
	ev := &engagementsv1.EngagementEvent{Envelope: events.NewEnvelope(ctx, eventID)}
	if like {
		ev.Payload = &engagementsv1.EngagementEvent_Liked{Liked: &engagementsv1.TweetLiked{
			TweetId: tweetID, UserId: userID, AuthorId: authorID,
		}}
	} else {
		ev.Payload = &engagementsv1.EngagementEvent_Unliked{Unliked: &engagementsv1.TweetUnliked{
			TweetId: tweetID, UserId: userID, AuthorId: authorID,
		}}
	}
	if err := events.Emit(ctx, tx, eventID, "engagements.v1", events.Key(tweetID), ev); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
