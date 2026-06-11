// Package timeline implements the hybrid home-timeline read (ARCHITECTURE.md
// §2.1): merge the fan-out list (tl:) with followed celebrities' own streams
// (utl:) by Snowflake id, hydrate, paginate by cursor, rebuild-on-miss from PG.
package timeline

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/fonvacano/yaxter/internal/tweets"
	"github.com/fonvacano/yaxter/pkg/redisx"
)

const (
	tlCap         = 800
	celebCap      = 1000 // max followed celebrities considered at read time
	celebTTL      = 10 * time.Minute
	celebSentinel = "_" // cached even when empty (anti-stampede), filtered out
	hydrateTTL    = 5 * time.Second
)

func tlKey(uid int64) string     { return fmt.Sprintf("tl:%d", uid) }
func utlKey(uid int64) string    { return fmt.Sprintf("utl:%d", uid) }
func celebsKey(uid int64) string { return fmt.Sprintf("celebs:%d", uid) }

// ErrUserNotFound is returned by Profile when the username does not exist.
var ErrUserNotFound = errors.New("timeline: user not found")

type Service struct {
	db        *pgxpool.Pool
	rdb       *redis.Client
	tweets    *tweets.Service
	threshold int
	loader    *redisx.Loader[tweets.HydratedTweet]
}

func NewService(db *pgxpool.Pool, rdb *redis.Client, tweetsSvc *tweets.Service, threshold int) (*Service, error) {
	loader, err := redisx.NewLoader[tweets.HydratedTweet](5000, hydrateTTL)
	if err != nil {
		return nil, fmt.Errorf("timeline: loader: %w", err)
	}
	return &Service{
		db:        db,
		rdb:       rdb,
		tweets:    tweetsSvc,
		threshold: threshold,
		loader:    loader,
	}, nil
}

// Home returns the reader's hybrid home timeline, newest-first, paginated by
// the Snowflake-id cursor (0 = first page).
func (s *Service) Home(ctx context.Context, uid, cursor int64, limit int) ([]tweets.HydratedTweet, *int64, error) {
	ids, err := s.homeIDs(ctx, uid)
	if err != nil {
		return nil, nil, err
	}
	return s.hydratePage(ctx, ids, cursor, limit)
}

// homeIDs returns the merged, de-duplicated, newest-first id list for uid.
func (s *Service) homeIDs(ctx context.Context, uid int64) ([]int64, error) {
	tlIDs, err := s.rdb.LRange(ctx, tlKey(uid), 0, -1).Result()
	if err != nil {
		return nil, err
	}
	var ids []int64
	if len(tlIDs) == 0 {
		rebuilt, err := s.rebuild(ctx, uid)
		if err != nil {
			return nil, err
		}
		ids = append(ids, rebuilt...)
	} else {
		ids = append(ids, parseIDs(tlIDs)...)
	}

	celebs, err := s.followedCelebrities(ctx, uid)
	if err != nil {
		return nil, err
	}
	for _, c := range celebs {
		raw, err := s.rdb.LRange(ctx, utlKey(c), 0, -1).Result()
		if err != nil {
			return nil, err
		}
		ids = append(ids, parseIDs(raw)...)
	}
	return mergeDesc(ids), nil
}

// rebuild reconstructs the fan-out list from PG (sub-threshold followees) and
// warms tl:{uid}. Returns the rebuilt id list newest-first.
func (s *Service) rebuild(ctx context.Context, uid int64) ([]int64, error) {
	rows, err := s.db.Query(ctx, `
		SELECT t.id
		FROM tweets t
		JOIN follows f ON f.followee_id = t.author_id
		JOIN users u ON u.id = t.author_id
		WHERE f.follower_id = $1 AND u.followers_count < $2
		ORDER BY t.id DESC
		LIMIT $3`, uid, s.threshold, tlCap)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		// RPUSH in newest-first order so index 0 stays newest.
		args := make([]interface{}, len(ids))
		for i, id := range ids {
			args[i] = id
		}
		pipe := s.rdb.Pipeline()
		pipe.RPush(ctx, tlKey(uid), args...)
		pipe.LTrim(ctx, tlKey(uid), 0, tlCap-1)
		if _, err := pipe.Exec(ctx); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

// followedCelebrities returns the reader's followed users over the threshold,
// cached in celebs:{uid} for 10m (with an empty-set sentinel).
func (s *Service) followedCelebrities(ctx context.Context, uid int64) ([]int64, error) {
	members, err := s.rdb.SMembers(ctx, celebsKey(uid)).Result()
	if err != nil {
		return nil, err
	}
	if len(members) > 0 {
		return parseCelebMembers(members), nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT u.id
		FROM follows f
		JOIN users u ON u.id = f.followee_id
		WHERE f.follower_id = $1 AND u.followers_count >= $2
		LIMIT $3`, uid, s.threshold, celebCap)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	add := []interface{}{celebSentinel}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
		add = append(add, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	pipe := s.rdb.Pipeline()
	pipe.SAdd(ctx, celebsKey(uid), add...)
	pipe.Expire(ctx, celebsKey(uid), celebTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	return ids, nil
}

// hydratePage applies the cursor, hydrates up to limit tweets (skipping ids
// missing from PG — deviation #2), and computes the next cursor.
func (s *Service) hydratePage(ctx context.Context, ids []int64, cursor int64, limit int) ([]tweets.HydratedTweet, *int64, error) {
	out := make([]tweets.HydratedTweet, 0, limit)
	var next *int64
	for _, id := range ids {
		if cursor != 0 && id >= cursor {
			continue
		}
		if len(out) == limit {
			n := out[len(out)-1].ID // cursor = last item on this page (next page: id < cursor)
			next = &n
			break
		}
		ht, err := s.loader.Get(ctx, fmt.Sprintf("tw:%d", id), func(ctx context.Context) (tweets.HydratedTweet, error) {
			return s.tweets.Get(ctx, id)
		})
		if err != nil {
			if isNotFound(err) {
				continue // deleted; skip (defense-in-depth)
			}
			return nil, nil, err
		}
		out = append(out, ht)
	}
	return out, next, nil
}

// Profile returns a user's own tweets newest-first, cursor-paginated.
func (s *Service) Profile(ctx context.Context, username string, cursor int64, limit int) ([]tweets.HydratedTweet, *int64, error) {
	var authorID int64
	err := s.db.QueryRow(ctx, `SELECT id FROM users WHERE username = $1`, username).Scan(&authorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrUserNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT id FROM tweets
		WHERE author_id = $1 AND ($2 = 0 OR id < $2)
		ORDER BY id DESC
		LIMIT $3`, authorID, cursor, limit+1)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var next *int64
	if len(ids) > limit {
		n := ids[limit-1]
		next = &n
		ids = ids[:limit]
	}
	out := make([]tweets.HydratedTweet, 0, len(ids))
	for _, id := range ids {
		ht, err := s.tweets.Get(ctx, id)
		if err != nil {
			if isNotFound(err) {
				continue
			}
			return nil, nil, err
		}
		out = append(out, ht)
	}
	return out, next, nil
}

func isNotFound(err error) bool { return err == tweets.ErrNotFound }

func parseIDs(raw []string) []int64 {
	ids := make([]int64, 0, len(raw))
	for _, s := range raw {
		var id int64
		if _, err := fmt.Sscan(s, &id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func parseCelebMembers(members []string) []int64 {
	ids := make([]int64, 0, len(members))
	for _, m := range members {
		if m == celebSentinel {
			continue
		}
		var id int64
		if _, err := fmt.Sscan(m, &id); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// mergeDesc de-duplicates and sorts ids descending (Snowflake = time order).
func mergeDesc(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	uniq := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	sort.Slice(uniq, func(i, j int) bool { return uniq[i] > uniq[j] })
	return uniq
}
