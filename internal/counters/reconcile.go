package counters

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Reconcile recomputes likes_count from the likes table and corrects both PG and the hot hash.
func Reconcile(ctx context.Context, db *pgxpool.Pool, rdb *redis.Client) error {
	rows, err := db.Query(ctx, `
		WITH truth AS (
			SELECT tweet_id, count(*) AS n FROM likes
			GROUP BY tweet_id
		)
		UPDATE tweets t SET likes_count = truth.n
		FROM truth
		WHERE t.id = truth.tweet_id AND t.likes_count <> truth.n
		RETURNING t.id, t.likes_count`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var likes int
		if err := rows.Scan(&id, &likes); err != nil {
			return err
		}
		rdb.HSet(ctx, fmt.Sprintf("cnt:%d", id), "likes", likes)
	}
	return rows.Err()
}
