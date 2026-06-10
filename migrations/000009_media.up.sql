CREATE TABLE media (
    id           BIGINT PRIMARY KEY,              -- snowflake
    owner_id     BIGINT NOT NULL,
    content_type TEXT   NOT NULL,
    size_bytes   BIGINT NOT NULL,
    status       TEXT   NOT NULL DEFAULT 'pending',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    ready_at     TIMESTAMPTZ
);
CREATE INDEX media_owner_id_idx ON media (owner_id);
