package tweets

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const utlCap = 200

func tweetKey(id int64) string   { return fmt.Sprintf("tw:%d", id) }
func utlKey(author int64) string { return fmt.Sprintf("utl:%d", author) }

func appendUTL(ctx context.Context, rdb *redis.Client, author, tweetID int64) {
	pipe := rdb.Pipeline()
	pipe.LPush(ctx, utlKey(author), tweetID)
	pipe.LTrim(ctx, utlKey(author), 0, utlCap-1)
	_, _ = pipe.Exec(ctx)
}

func cacheTweet(ctx context.Context, rdb *redis.Client, tw Tweet) {
	raw, err := json.Marshal(tw)
	if err != nil {
		return
	}
	rdb.Set(ctx, tweetKey(tw.ID), raw, time.Hour)
}

func dropTweetCaches(ctx context.Context, rdb *redis.Client, author, tweetID int64) {
	rdb.Del(ctx, tweetKey(tweetID))
	rdb.LRem(ctx, utlKey(author), 0, tweetID)
}
