package tweets

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	tweetev "github.com/fonvacano/yaxter/gen/yaxter/events/tweets/v1"
	"github.com/fonvacano/yaxter/internal/events"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

// ErrNotFound is returned when a tweet or related entity does not exist.
var ErrNotFound = errors.New("tweets: not found")

// ErrForbidden is returned when the requesting user does not own the resource.
var ErrForbidden = errors.New("tweets: forbidden")

// Tweet is the in-memory representation of a tweet row.
type Tweet struct {
	ID            int64
	AuthorID      int64
	Text          string
	RetweetOfID   int64
	MediaIDs      []int64
	LikesCount    int
	RetweetsCount int
	CreatedAt     time.Time
}

// Service implements tweet write and read operations.
type Service struct {
	db  *pgxpool.Pool
	rdb *redis.Client
	ids *snowflake.Generator
}

// NewService creates a Service.
func NewService(db *pgxpool.Pool, rdb *redis.Client, ids *snowflake.Generator) *Service {
	return &Service{db: db, rdb: rdb, ids: ids}
}

// Create inserts a tweet row and emits TweetCreated into the outbox.
func (s *Service) Create(ctx context.Context, authorID int64, text string, mediaIDs []int64, retweetOf int64) (*Tweet, error) {
	id := s.ids.Next()
	var mediaJSON []byte
	if len(mediaIDs) > 0 {
		var err error
		mediaJSON, err = json.Marshal(mediaIDs)
		if err != nil {
			return nil, err
		}
	}
	var retweetOfPtr *int64
	if retweetOf != 0 {
		retweetOfPtr = &retweetOf
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var createdAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO tweets (id, author_id, text, retweet_of_id, media)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at`,
		id, authorID, text, retweetOfPtr, nullableJSON(mediaJSON),
	).Scan(&createdAt)
	if err != nil {
		return nil, err
	}

	ev := &tweetev.TweetEvent{
		Envelope: events.NewEnvelope(ctx, id),
		Payload: &tweetev.TweetEvent_Created{Created: &tweetev.TweetCreated{
			TweetId:  id,
			AuthorId: authorID,
			Text:     text,
		}},
	}
	if err := events.Emit(ctx, tx, id, "tweets.v1", events.Key(authorID), ev); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	tw := &Tweet{
		ID:        id,
		AuthorID:  authorID,
		Text:      text,
		MediaIDs:  mediaIDs,
		CreatedAt: createdAt,
	}
	if retweetOf != 0 {
		tw.RetweetOfID = retweetOf
	}
	return tw, nil
}

// Delete removes a tweet (owner check) and emits TweetDeleted.
func (s *Service) Delete(ctx context.Context, userID, tweetID int64) error {
	var authorID int64
	err := s.db.QueryRow(ctx, `SELECT author_id FROM tweets WHERE id = $1`, tweetID).Scan(&authorID)
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

	evID := s.ids.Next()
	ev := &tweetev.TweetEvent{
		Envelope: events.NewEnvelope(ctx, evID),
		Payload: &tweetev.TweetEvent_Deleted{Deleted: &tweetev.TweetDeleted{
			TweetId:  tweetID,
			AuthorId: authorID,
		}},
	}
	if err := events.Emit(ctx, tx, evID, "tweets.v1", events.Key(authorID), ev); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func nullableJSON(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}

