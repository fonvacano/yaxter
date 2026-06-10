-- tweets sharded by author_id: profile timeline is single-shard. The
-- (author_id, id DESC) index serves cursor pagination index-only (§2.6).
CREATE TABLE tweets (
    id             BIGINT PRIMARY KEY,            -- snowflake
    author_id      BIGINT NOT NULL,
    text           VARCHAR(280) NOT NULL,
    retweet_of_id  BIGINT,
    media          JSONB,
    likes_count    INT NOT NULL DEFAULT 0,        -- denormalized, eventual (§2.7)
    retweets_count INT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX tweets_author_id_id_idx ON tweets (author_id, id DESC);

CREATE TABLE likes (
    user_id    BIGINT NOT NULL,
    tweet_id   BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, tweet_id)               -- idempotent ON CONFLICT DO NOTHING
);
CREATE INDEX likes_tweet_id_idx ON likes (tweet_id); -- nightly reconcile scans
