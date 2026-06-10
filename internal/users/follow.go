package users

import (
	"context"
	"fmt"

	followsv1 "github.com/fonvacano/yaxter/gen/yaxter/events/follows/v1"
	"github.com/fonvacano/yaxter/internal/events"
)

// Follow writes both edge tables, bumps both counters, and emits exactly one
// FollowChanged — all in one transaction (§2.2). Double-follow is a no-op.
func (s *Service) Follow(ctx context.Context, followerID int64, followeeUsername string) error {
	return s.setFollow(ctx, followerID, followeeUsername, true)
}

// Unfollow is the mirror image; unfollowing a non-followee is a no-op.
func (s *Service) Unfollow(ctx context.Context, followerID int64, followeeUsername string) error {
	return s.setFollow(ctx, followerID, followeeUsername, false)
}

func (s *Service) setFollow(ctx context.Context, followerID int64, followeeUsername string, follow bool) error {
	followee, err := s.GetByUsername(ctx, followeeUsername)
	if err != nil {
		return err
	}
	if followee.ID == followerID {
		return ErrSelfFollow
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var changed bool
	if follow {
		tag, err := tx.Exec(ctx, `
			INSERT INTO follows (follower_id, followee_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, followerID, followee.ID)
		if err != nil {
			return err
		}
		changed = tag.RowsAffected() == 1
		if changed {
			if _, err := tx.Exec(ctx, `
				INSERT INTO followers (followee_id, follower_id) VALUES ($1, $2)
				ON CONFLICT DO NOTHING`, followee.ID, followerID); err != nil {
				return err
			}
		}
	} else {
		tag, err := tx.Exec(ctx, `
			DELETE FROM follows WHERE follower_id = $1 AND followee_id = $2`,
			followerID, followee.ID)
		if err != nil {
			return err
		}
		changed = tag.RowsAffected() == 1
		if changed {
			if _, err := tx.Exec(ctx, `
				DELETE FROM followers WHERE followee_id = $1 AND follower_id = $2`,
				followee.ID, followerID); err != nil {
				return err
			}
		}
	}
	if !changed {
		return nil
	}

	delta := 1
	if !follow {
		delta = -1
	}
	if _, err := tx.Exec(ctx,
		`UPDATE users SET followers_count = followers_count + $2 WHERE id = $1`,
		followee.ID, delta); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE users SET following_count = following_count + $2 WHERE id = $1`,
		followerID, delta); err != nil {
		return err
	}

	eventID := s.ids.Next()
	ev := &followsv1.FollowEvent{
		Envelope: events.NewEnvelope(ctx, eventID),
		Payload: &followsv1.FollowEvent_FollowChanged{FollowChanged: &followsv1.FollowChanged{
			FollowerId: followerID,
			FolloweeId: followee.ID,
			Following:  follow,
		}},
	}
	if err := events.Emit(ctx, tx, eventID, "follows.v1", events.Key(followee.ID), ev); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	s.rdb.Del(ctx,
		fmt.Sprintf("usr:%d", followee.ID),
		fmt.Sprintf("usr:%d", followerID))
	if followee.FollowersCount+delta >= s.celebrityThreshold || followee.FollowersCount >= s.celebrityThreshold {
		s.rdb.Del(ctx, fmt.Sprintf("celebs:%d", followerID))
	}
	return nil
}
