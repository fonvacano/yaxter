-- Sharded by user_id (§2.2). UNIQUE on username/email is per-physical-shard;
-- global uniqueness is enforced via the global_* lookup tables below, which
-- the demo colocates on the single cluster.
CREATE TABLE users (
    id              BIGINT PRIMARY KEY,
    username        CITEXT NOT NULL UNIQUE,
    email           CITEXT NOT NULL UNIQUE,
    pass_hash       TEXT,                         -- NULL for OAuth-only accounts
    bio             TEXT   NOT NULL DEFAULT '',
    avatar_key      TEXT,
    followers_count INT    NOT NULL DEFAULT 0,
    following_count INT    NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE identities (
    user_id          BIGINT NOT NULL REFERENCES users (id),
    provider         TEXT   NOT NULL,
    provider_user_id TEXT   NOT NULL,
    email            CITEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, provider),
    UNIQUE (provider, provider_user_id)
);

-- Global lookup: (provider, provider_user_id) -> user, for OAuth login
-- before the user's shard is known (§2.2 "global lookup tables").
CREATE TABLE global_identities (
    provider         TEXT   NOT NULL,
    provider_user_id TEXT   NOT NULL,
    user_id          BIGINT NOT NULL,
    PRIMARY KEY (provider, provider_user_id)
);
