-- Security-relevant account events (§2.8: auto-link is audit-logged).
CREATE TABLE audit_log (
    id         BIGINT PRIMARY KEY,             -- snowflake
    user_id    BIGINT NOT NULL,
    action     TEXT   NOT NULL,                -- e.g. oauth_link, oauth_unlink
    detail     JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_user_id_id_idx ON audit_log (user_id, id DESC);
