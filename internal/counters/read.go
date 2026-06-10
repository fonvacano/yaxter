package counters

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Read returns (likes, retweets): cnt:<id> hash first, PG columns on miss (warming the hash).
func Read(ctx context.Context, rdb *redis.Client, db *pgxpool.Pool, tweetID int64) (int, int, error) {
	key := fmt.Sprintf("cnt:%d", tweetID)
	vals, err := rdb.HGetAll(ctx, key).Result()
	if err == nil && len(vals) > 0 {
		var likes, retweets int
		_, _ = fmt.Sscan(vals["likes"], &likes)
		_, _ = fmt.Sscan(vals["retweets"], &retweets)
		return likes, retweets, nil
	}
	var likes, retweets int
	if err := db.QueryRow(ctx,
		`SELECT likes_count, retweets_count FROM tweets WHERE id = $1`, tweetID).
		Scan(&likes, &retweets); err != nil {
		return 0, 0, err
	}
	pipe := rdb.Pipeline()
	pipe.HSet(ctx, key, "likes", likes, "retweets", retweets)
	pipe.Expire(ctx, key, 24*time.Hour)
	_, _ = pipe.Exec(ctx)
	return likes, retweets, nil
}
