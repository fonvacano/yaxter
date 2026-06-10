-- follows sharded by follower_id ("who do I follow"); followers is the
-- duplicated reverse edge sharded by followee_id ("who follows X"), kept in
-- sync via FollowChanged (§2.2). No FKs: edges cross shards.
CREATE TABLE follows (
    follower_id BIGINT NOT NULL,
    followee_id BIGINT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (follower_id, followee_id)
);

CREATE TABLE followers (
    followee_id BIGINT NOT NULL,
    follower_id BIGINT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (followee_id, follower_id)
);
