// Package tweets implements the tweet/retweet write path (ARCHITECTURE.md §2.1).
package tweets

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	engagementsv1 "github.com/fonvacano/yaxter/gen/yaxter/events/engagements/v1"
	tweetsv1 "github.com/fonvacano/yaxter/gen/yaxter/events/tweets/v1"
	"github.com/fonvacano/yaxter/internal/events"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

var (
	ErrValidation    = errors.New("tweets: validation failed")
	ErrNotFound      = errors.New("tweets: not found")
	ErrForbidden     = errors.New("tweets: not the author")
	ErrMediaNotReady = errors.New("tweets: media not ready")
)

// Tweet is the canonical in-memory representation of a tweet row.
type Tweet struct {
	ID            int64     `json:"id"`
	AuthorID      int64     `json:"author_id"`
	Text          string    `json:"text"`
	RetweetOfID   int64     `json:"retweet_of_id,omitempty"`
	MediaIDs      []int64   `json:"media_ids,omitempty"`
	LikesCount    int       `json:"likes_count"`
	RetweetsCount int       `json:"retweets_count"`
	CreatedAt     time.Time `json:"created_at"`
}

// Service implements the tweet write path.
type Service struct {
	db  *pgxpool.Pool
	rdb *redis.Client
	ids *snowflake.Generator
}

// NewService constructs a Service.
func NewService(db *pgxpool.Pool, rdb *redis.Client, ids *snowflake.Generator) *Service {
	return &Service{db: db, rdb: rdb, ids: ids}
}

// Create creates a new tweet (or retweet when retweetOfID != 0).
// Retweets are flattened: retweeting a retweet points to the original.
// On success, the tweet is appended to the author's user-timeline-list in
// Redis and cached by ID. The created event (and, for retweets, the
// engagement event) are written to the outbox in the same transaction.
func (s *Service) Create(ctx context.Context, authorID int64, text string, mediaIDs []int64, retweetOfID int64) (Tweet, error) {
	if utf8.RuneCountInString(text) > 280 {
		return Tweet{}, ErrValidation
	}
	if text == "" && retweetOfID == 0 {
		return Tweet{}, ErrValidation
	}
	if len(mediaIDs) > 4 {
		return Tweet{}, ErrValidation
	}
	if len(mediaIDs) > 0 {
		var ready int
		if err := s.db.QueryRow(ctx, `
			SELECT count(*) FROM media
			WHERE id = ANY($1) AND owner_id = $2 AND status = 'ready'`,
			mediaIDs, authorID).Scan(&ready); err != nil {
			return Tweet{}, err
		}
		if ready != len(mediaIDs) {
			return Tweet{}, ErrMediaNotReady
		}
	}

	var origAuthorID int64
	if retweetOfID != 0 {
		var origRetweetOf *int64
		err := s.db.QueryRow(ctx,
			`SELECT author_id, retweet_of_id FROM tweets WHERE id = $1`,
			retweetOfID).Scan(&origAuthorID, &origRetweetOf)
		if errors.Is(err, pgx.ErrNoRows) {
			return Tweet{}, ErrNotFound
		}
		if err != nil {
			return Tweet{}, err
		}
		// Flatten: if the target is itself a retweet, point to the original.
		if origRetweetOf != nil {
			retweetOfID = *origRetweetOf
			if err := s.db.QueryRow(ctx,
				`SELECT author_id FROM tweets WHERE id = $1`, retweetOfID).
				Scan(&origAuthorID); err != nil {
				return Tweet{}, err
			}
		}
	}

	var followersCount int32
	if err := s.db.QueryRow(ctx,
		`SELECT followers_count FROM users WHERE id = $1`, authorID).
		Scan(&followersCount); err != nil {
		return Tweet{}, err
	}

	tw := Tweet{
		ID:          s.ids.Next(),
		AuthorID:    authorID,
		Text:        text,
		RetweetOfID: retweetOfID,
		MediaIDs:    mediaIDs,
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Tweet{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var retweetOf *int64
	if retweetOfID != 0 {
		retweetOf = &retweetOfID
	}
	var mediaJSON []byte
	if len(mediaIDs) > 0 {
		mediaJSON, err = json.Marshal(mediaIDs)
		if err != nil {
			return Tweet{}, err
		}
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO tweets (id, author_id, text, retweet_of_id, media)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at`,
		tw.ID, authorID, text, retweetOf, mediaJSON).Scan(&tw.CreatedAt); err != nil {
		return Tweet{}, err
	}

	createdID := s.ids.Next()
	created := &tweetsv1.TweetEvent{
		Envelope: events.NewEnvelope(ctx, createdID),
		Payload: &tweetsv1.TweetEvent_Created{Created: &tweetsv1.TweetCreated{
			TweetId:              tw.ID,
			AuthorId:             authorID,
			Text:                 text,
			RetweetOfId:          retweetOfID,
			MediaIds:             mediaIDs,
			AuthorFollowersCount: followersCount,
		}},
	}
	if err := events.Emit(ctx, tx, createdID, "tweets.v1", events.Key(authorID), created); err != nil {
		return Tweet{}, err
	}

	if retweetOfID != 0 {
		engID := s.ids.Next()
		eng := &engagementsv1.EngagementEvent{
			Envelope: events.NewEnvelope(ctx, engID),
			Payload: &engagementsv1.EngagementEvent_Retweeted{Retweeted: &engagementsv1.TweetRetweeted{
				TweetId:   retweetOfID,
				RetweetId: tw.ID,
				UserId:    authorID,
				AuthorId:  origAuthorID,
			}},
		}
		if err := events.Emit(ctx, tx, engID, "engagements.v1", events.Key(retweetOfID), eng); err != nil {
			return Tweet{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Tweet{}, err
	}

	appendUTL(ctx, s.rdb, authorID, tw.ID)
	cacheTweet(ctx, s.rdb, tw)
	return tw, nil
}

// Delete removes a tweet owned by userID and emits a TweetDeleted event.
// If the tweet is a retweet, an Unretweeted engagement event is also emitted.
func (s *Service) Delete(ctx context.Context, userID, tweetID int64) error {
	var authorID int64
	var retweetOf *int64
	err := s.db.QueryRow(ctx,
		`SELECT author_id, retweet_of_id FROM tweets WHERE id = $1`, tweetID).
		Scan(&authorID, &retweetOf)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if authorID != userID {
		return ErrForbidden
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx, `DELETE FROM tweets WHERE id = $1`, tweetID); err != nil {
		return err
	}

	delID := s.ids.Next()
	deleted := &tweetsv1.TweetEvent{
		Envelope: events.NewEnvelope(ctx, delID),
		Payload: &tweetsv1.TweetEvent_Deleted{Deleted: &tweetsv1.TweetDeleted{
			TweetId:  tweetID,
			AuthorId: authorID,
		}},
	}
	if err := events.Emit(ctx, tx, delID, "tweets.v1", events.Key(authorID), deleted); err != nil {
		return err
	}

	if retweetOf != nil {
		engID := s.ids.Next()
		eng := &engagementsv1.EngagementEvent{
			Envelope: events.NewEnvelope(ctx, engID),
			Payload: &engagementsv1.EngagementEvent_Unretweeted{Unretweeted: &engagementsv1.TweetUnretweeted{
				TweetId:   *retweetOf,
				RetweetId: tweetID,
				UserId:    userID,
			}},
		}
		if err := events.Emit(ctx, tx, engID, "engagements.v1", events.Key(*retweetOf), eng); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	dropTweetCaches(ctx, s.rdb, authorID, tweetID)
	return nil
}
