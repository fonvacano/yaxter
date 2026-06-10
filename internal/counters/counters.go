// Package counters implements §2.7: buffered write-behind counters.
package counters

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"

	engagementsv1 "github.com/fonvacano/yaxter/gen/yaxter/events/engagements/v1"
	"github.com/fonvacano/yaxter/pkg/kafkax"
	"github.com/fonvacano/yaxter/pkg/redisx"

	"github.com/twmb/franz-go/pkg/kgo"
)

type delta struct{ likes, retweets int }

// Counters accumulates engagement deltas in memory and flushes them to PG in batches.
type Counters struct {
	db         *pgxpool.Pool
	rdb        *redis.Client
	flushAfter int
	flushEvery time.Duration

	mu      sync.Mutex
	deltas  map[int64]delta
	pending int
	flushes int
}

// New creates a Counters. flushAfter: flush after this many events; flushEvery: tick-based flush.
func New(db *pgxpool.Pool, rdb *redis.Client, flushAfter int, flushEvery time.Duration) *Counters {
	return &Counters{
		db: db, rdb: rdb,
		flushAfter: flushAfter, flushEvery: flushEvery,
		deltas: make(map[int64]delta),
	}
}

// HandleEvent deduplicates using Redis SETNX, updates the hot hash, and accumulates a delta.
func (c *Counters) HandleEvent(ctx context.Context, ev *engagementsv1.EngagementEvent) error {
	first, err := redisx.Once(ctx, c.rdb,
		fmt.Sprintf("evt:%d", ev.GetEnvelope().GetEventId()), 24*time.Hour)
	if err != nil {
		return err
	}
	if !first {
		return nil
	}

	var tweetID int64
	var d delta
	switch p := ev.Payload.(type) {
	case *engagementsv1.EngagementEvent_Liked:
		tweetID, d.likes = p.Liked.GetTweetId(), 1
	case *engagementsv1.EngagementEvent_Unliked:
		tweetID, d.likes = p.Unliked.GetTweetId(), -1
	case *engagementsv1.EngagementEvent_Retweeted:
		tweetID, d.retweets = p.Retweeted.GetTweetId(), 1
	case *engagementsv1.EngagementEvent_Unretweeted:
		tweetID, d.retweets = p.Unretweeted.GetTweetId(), -1
	default:
		return nil
	}

	key := fmt.Sprintf("cnt:%d", tweetID)
	pipe := c.rdb.Pipeline()
	if d.likes != 0 {
		pipe.HIncrBy(ctx, key, "likes", int64(d.likes))
	}
	if d.retweets != 0 {
		pipe.HIncrBy(ctx, key, "retweets", int64(d.retweets))
	}
	pipe.Expire(ctx, key, 24*time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	cur := c.deltas[tweetID]
	cur.likes += d.likes
	cur.retweets += d.retweets
	c.deltas[tweetID] = cur
	c.pending++
	full := c.pending >= c.flushAfter
	c.mu.Unlock()

	if full {
		return c.Flush(ctx)
	}
	return nil
}

// Flush writes accumulated deltas to PG in a single batch and resets the buffer.
func (c *Counters) Flush(ctx context.Context) error {
	c.mu.Lock()
	if c.pending == 0 {
		c.mu.Unlock()
		return nil
	}
	batchDeltas := c.deltas
	c.deltas = make(map[int64]delta)
	c.pending = 0
	c.flushes++
	c.mu.Unlock()

	batch := &pgx.Batch{}
	for tweetID, d := range batchDeltas {
		batch.Queue(`
			UPDATE tweets SET
				likes_count    = likes_count + $2,
				retweets_count = retweets_count + $3
			WHERE id = $1`, tweetID, d.likes, d.retweets)
	}
	return c.db.SendBatch(ctx, batch).Close()
}

// FlushCount returns the number of Flush calls so far (for tests).
func (c *Counters) FlushCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.flushes
}

// Run is a long-running loop that flushes on a ticker until ctx is done.
func (c *Counters) Run(ctx context.Context) {
	t := time.NewTicker(c.flushEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = c.Flush(flushCtx)
			cancel()
			return
		case <-t.C:
			_ = c.Flush(ctx)
		}
	}
}

// HandleRecord decodes a raw engagements.v1 Kafka record and applies it.
func (c *Counters) HandleRecord(ctx context.Context, rec *kgo.Record) error {
	var ev engagementsv1.EngagementEvent
	if err := proto.Unmarshal(rec.Value, &ev); err != nil {
		return err
	}
	return c.HandleEvent(ctx, &ev)
}

// compile-time check: HandleRecord satisfies kafkax.Handler
var _ kafkax.Handler = (*Counters)(nil).HandleRecord
