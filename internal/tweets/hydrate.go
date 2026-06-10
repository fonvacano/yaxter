package tweets

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/fonvacano/yaxter/internal/counters"
)

// HydratedTweet extends Tweet with author info resolved at read time.
type HydratedTweet struct {
	Tweet
	AuthorUsername  string
	AuthorAvatarKey *string
}

// Get loads a tweet with author info and live counter values (Redis read-through §2.7).
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
	// counters served from hot hash, PG as fallback (§2.7 step 3)
	likes, retweets, cErr := counters.Read(ctx, s.rdb, s.db, tweetID)
	if cErr == nil {
		h.LikesCount, h.RetweetsCount = likes, retweets
	}
	return h, nil
}
