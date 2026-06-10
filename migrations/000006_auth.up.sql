-- Rotating refresh tokens with reuse-detection: reuse of a rotated token
-- revokes the whole family_id (§2.8).
CREATE TABLE refresh_tokens (
    id         BIGINT PRIMARY KEY,                -- snowflake
    user_id    BIGINT NOT NULL,
    family_id  BIGINT NOT NULL,
    token_hash TEXT   NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);
CREATE INDEX refresh_tokens_user_id_idx ON refresh_tokens (user_id);

-- Durable tier of the idempotency store (Redis is the hot tier, §2.3).
CREATE TABLE idempotency (
    key           UUID PRIMARY KEY,
    user_id       BIGINT NOT NULL,
    response_hash TEXT   NOT NULL,
    expires_at    TIMESTAMPTZ NOT NULL
);
