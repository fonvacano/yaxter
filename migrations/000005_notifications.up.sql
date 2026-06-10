CREATE TABLE notifications (
    id         BIGINT PRIMARY KEY,                -- snowflake
    user_id    BIGINT NOT NULL,
    kind       TEXT   NOT NULL,
    actor_id   BIGINT NOT NULL,
    subject_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at    TIMESTAMPTZ
);
CREATE INDEX notifications_user_id_id_idx ON notifications (user_id, id DESC);
CREATE INDEX notifications_unread_idx ON notifications (user_id) WHERE read_at IS NULL;
