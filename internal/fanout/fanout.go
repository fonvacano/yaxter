// Package fanout implements worker:fanout — the write-path consumer that
// pushes new tweet ids into followers' Redis home-timeline lists, skipping
// celebrity authors (merged at read time instead). ARCHITECTURE.md §2.1.
package fanout

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	tweetsv1 "github.com/fonvacano/yaxter/gen/yaxter/events/tweets/v1"
	"github.com/fonvacano/yaxter/pkg/redisx"
)

const (
	tlCap        = 800  // home-timeline list cap (ARCHITECTURE.md §2.3)
	followerPage = 1000 // follower paging size (ARCHITECTURE.md §2.1)
)

func tlKey(uid int64) string { return fmt.Sprintf("tl:%d", uid) }

type Fanout struct {
	db        *pgxpool.Pool
	rdb       *redis.Client
	threshold int
	metrics   *Metrics
}

func New(db *pgxpool.Pool, rdb *redis.Client, threshold int, metrics *Metrics) *Fanout {
	return &Fanout{db: db, rdb: rdb, threshold: threshold, metrics: metrics}
}

// HandleRecord decodes a tweets.v1 record and dispatches it.
func (f *Fanout) HandleRecord(ctx context.Context, rec *kgo.Record) error {
	var ev tweetsv1.TweetEvent
	if err := proto.Unmarshal(rec.Value, &ev); err != nil {
		return fmt.Errorf("fanout: unmarshal: %w", err)
	}
	return f.HandleEvent(ctx, &ev)
}

// HandleEvent fans out a single tweet event. Idempotent on redelivery via
// event_id dedupe; per-aggregate ordering is preserved by the Kafka key.
func (f *Fanout) HandleEvent(ctx context.Context, ev *tweetsv1.TweetEvent) error {
	eventID := ev.GetEnvelope().GetEventId()
	first, err := redisx.Once(ctx, f.rdb, fmt.Sprintf("evt:fanout:%d", eventID), 24*time.Hour)
	if err != nil {
		return fmt.Errorf("fanout: dedupe: %w", err)
	}
	if !first {
		return nil // already processed
	}
	if occurred := ev.GetEnvelope().GetOccurredAt(); occurred != nil {
		f.metrics.LagSeconds.Set(time.Since(occurred.AsTime()).Seconds())
	}
	switch p := ev.GetPayload().(type) {
	case *tweetsv1.TweetEvent_Created:
		return f.fanoutCreate(ctx, p.Created)
	case *tweetsv1.TweetEvent_Deleted:
		return f.fanoutDelete(ctx, p.Deleted)
	default:
		return nil
	}
}

func (f *Fanout) fanoutCreate(ctx context.Context, c *tweetsv1.TweetCreated) error {
	if int(c.GetAuthorFollowersCount()) >= f.threshold {
		f.metrics.Skipped.Inc() // celebrity — merged at read time
		return nil
	}
	if err := f.eachFollowerPage(ctx, c.GetAuthorId(), func(followers []int64) error {
		pipe := f.rdb.Pipeline()
		for _, follower := range followers {
			key := tlKey(follower)
			pipe.LPush(ctx, key, c.GetTweetId())
			pipe.LTrim(ctx, key, 0, tlCap-1)
		}
		_, err := pipe.Exec(ctx)
		return err
	}); err != nil {
		return err
	}
	f.metrics.Processed.Inc()
	return nil
}

// eachFollowerPage pages the reverse-edge `followers` table for authorID,
// paging by follower_id descending (index-only on the PK), invoking fn per page.
func (f *Fanout) eachFollowerPage(ctx context.Context, authorID int64, fn func([]int64) error) error {
	cursor := int64(0) // 0 = no cursor (first page)
	for {
		rows, err := f.db.Query(ctx, `
			SELECT follower_id FROM followers
			WHERE followee_id = $1 AND ($2 = 0 OR follower_id < $2)
			ORDER BY follower_id DESC
			LIMIT $3`, authorID, cursor, followerPage)
		if err != nil {
			return fmt.Errorf("fanout: load followers: %w", err)
		}
		page := make([]int64, 0, followerPage)
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			page = append(page, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(page) == 0 {
			return nil
		}
		if err := fn(page); err != nil {
			return err
		}
		if len(page) < followerPage {
			return nil
		}
		cursor = page[len(page)-1]
	}
}

// fanoutDelete best-effort removes the tweet id from sub-threshold authors'
// follower timelines (deviation #2). Over-threshold authors were never fanned
// out, so nothing to remove; the read path also skips ids missing from PG.
func (f *Fanout) fanoutDelete(ctx context.Context, d *tweetsv1.TweetDeleted) error {
	var followers int
	err := f.db.QueryRow(ctx,
		`SELECT followers_count FROM users WHERE id = $1`, d.GetAuthorId()).Scan(&followers)
	if err != nil {
		return fmt.Errorf("fanout: author lookup: %w", err)
	}
	if followers >= f.threshold {
		return nil // celebrity — never fanned out
	}
	return f.eachFollowerPage(ctx, d.GetAuthorId(), func(followers []int64) error {
		pipe := f.rdb.Pipeline()
		for _, follower := range followers {
			pipe.LRem(ctx, tlKey(follower), 0, d.GetTweetId())
		}
		_, err := pipe.Exec(ctx)
		return err
	})
}
