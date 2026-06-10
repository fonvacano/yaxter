package tweets

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

// HydratedTweet is a Tweet plus its author projection. The author comes from
// a local query (plan deviation #4 — no cross-module import); counters come
// from the PG columns here and are upgraded to the Redis read-through by the
// counters track (T1.4) — on merge conflict, the counters version wins.
type HydratedTweet struct {
	Tweet
	AuthorUsername  string
	AuthorAvatarKey *string
}

// Get fetches a single tweet by ID, joining the author's username and
// avatar_key from the users table. Returns ErrNotFound when absent.
func (s *Service) Get(ctx context.Context, tweetID int64) (HydratedTweet, error) {
	var h HydratedTweet
	var retweetOf *int64
	var mediaJSON []byte
	err := s.db.QueryRow(ctx, `
		SELECT t.id, t.author_id, t.text, t.retweet_of_id, t.media, t.created_at,
		       t.likes_count, t.retweets_count, u.username, u.avatar_key
		FROM tweets t JOIN users u ON u.id = t.author_id
		WHERE t.id = $1`, tweetID).
		Scan(&h.ID, &h.AuthorID, &h.Text, &retweetOf, &mediaJSON, &h.CreatedAt,
			&h.LikesCount, &h.RetweetsCount, &h.AuthorUsername, &h.AuthorAvatarKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return HydratedTweet{}, ErrNotFound
	}
	if err != nil {
		return HydratedTweet{}, err
	}
	if retweetOf != nil {
		h.RetweetOfID = *retweetOf
	}
	if len(mediaJSON) > 0 {
		_ = json.Unmarshal(mediaJSON, &h.MediaIDs)
	}
	return h, nil
}
