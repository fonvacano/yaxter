-- Outbox rows are written in the SAME tx as the domain row and deleted soon
-- after publish; fillfactor + aggressive autovacuum keep churn cheap (§2.4).
CREATE TABLE outbox (
    id           BIGINT PRIMARY KEY,              -- snowflake = publish order
    topic        TEXT  NOT NULL,
    key          TEXT  NOT NULL,
    payload      BYTEA NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
) WITH (fillfactor = 70);
ALTER TABLE outbox SET (
    autovacuum_vacuum_scale_factor = 0.01,
    autovacuum_vacuum_cost_delay = 0
);
CREATE INDEX outbox_unpublished_idx ON outbox (id) WHERE published_at IS NULL;

-- Snowflake worker-ID leases (§2.6). Must stay compatible with the
-- CREATE TABLE IF NOT EXISTS in pkg/snowflake/lease.go (T0.1).
CREATE TABLE IF NOT EXISTS node_ids (
    node_id      INT PRIMARY KEY,
    leased_by    TEXT NOT NULL,
    heartbeat_at TIMESTAMPTZ NOT NULL
);
