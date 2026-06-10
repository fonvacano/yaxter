package snowflake

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Lease holds a node ID claimed in the node_ids table (ARCHITECTURE.md §2.6).
// Callers must call Heartbeat at an interval well under ttl (e.g. ttl/3) for
// the life of the process.
type Lease struct {
	pool   *pgxpool.Pool
	nodeID int64
	owner  string
	ttl    time.Duration
}

const leaseDDL = `
CREATE TABLE IF NOT EXISTS node_ids (
    node_id      INT PRIMARY KEY,
    leased_by    TEXT        NOT NULL,
    heartbeat_at TIMESTAMPTZ NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL
)`

// claimSQL claims the lowest node id that is either unused or expired.
// expires_at is set at write time using the issuer's TTL, so expiry is
// checked against the row's own deadline — not the acquirer's TTL.
// The WHERE on the conflict update makes a non-expired row un-stealable.
const claimSQL = `
INSERT INTO node_ids (node_id, leased_by, heartbeat_at, expires_at)
SELECT n.id, $1, now(), now() + $2::interval
FROM generate_series(0, $3::int) AS n(id)
WHERE NOT EXISTS (
    SELECT 1 FROM node_ids t
    WHERE t.node_id = n.id AND t.expires_at > now()
)
ORDER BY n.id
LIMIT 1
ON CONFLICT (node_id) DO UPDATE
    SET leased_by    = EXCLUDED.leased_by,
        heartbeat_at = now(),
        expires_at   = now() + $2::interval
    WHERE node_ids.expires_at <= now()
RETURNING node_id`

// AcquireLease claims a free (or expired) node id, retrying a few times on
// claim races. ttl is how long the lease survives without a heartbeat.
func AcquireLease(ctx context.Context, pool *pgxpool.Pool, owner string, ttl time.Duration) (*Lease, error) {
	if _, err := pool.Exec(ctx, leaseDDL); err != nil {
		return nil, fmt.Errorf("snowflake lease: ensure table: %w", err)
	}
	interval := fmt.Sprintf("%d milliseconds", ttl.Milliseconds())
	for attempt := 0; attempt < 5; attempt++ {
		var nodeID int64
		err := pool.QueryRow(ctx, claimSQL, owner, interval, MaxNode).Scan(&nodeID)
		if err == nil {
			return &Lease{pool: pool, nodeID: nodeID, owner: owner, ttl: ttl}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("snowflake lease: claim: %w", err)
		}
		// Lost a race for that id — try again.
	}
	return nil, errors.New("snowflake lease: no node id claimable after 5 attempts")
}

// NodeID returns the claimed node id.
func (l *Lease) NodeID() int64 { return l.nodeID }

// Heartbeat extends the lease. An error here means the lease may have been
// stolen; callers should treat repeated failures as fatal.
func (l *Lease) Heartbeat(ctx context.Context) error {
	interval := fmt.Sprintf("%d milliseconds", l.ttl.Milliseconds())
	tag, err := l.pool.Exec(ctx,
		`UPDATE node_ids SET heartbeat_at = now(), expires_at = now() + $3::interval WHERE node_id = $1 AND leased_by = $2`,
		l.nodeID, l.owner, interval)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("snowflake lease: node %d no longer owned by %s", l.nodeID, l.owner)
	}
	return nil
}

// Release frees the node id immediately (best-effort, for graceful shutdown).
func (l *Lease) Release(ctx context.Context) error {
	_, err := l.pool.Exec(ctx,
		`DELETE FROM node_ids WHERE node_id = $1 AND leased_by = $2`,
		l.nodeID, l.owner)
	return err
}
