// Package relay implements the transactional-outbox relay worker
// (ARCHITECTURE.md §2.4): the only producer to Kafka in the system.
package relay

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AcquireLeadership blocks until the session-level advisory lock lockID is
// held (one active relay per physical shard) or ctx ends. The returned
// release func unlocks and returns the connection; losing the underlying
// connection releases the lock automatically (crash safety).
func AcquireLeadership(ctx context.Context, pool *pgxpool.Pool, lockID int64, retry time.Duration) (func(), error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	for {
		var got bool
		if err := conn.QueryRow(ctx,
			`SELECT pg_try_advisory_lock($1)`, lockID).Scan(&got); err != nil {
			conn.Release()
			return nil, err
		}
		if got {
			release := func() {
				_, _ = conn.Exec(context.Background(),
					`SELECT pg_advisory_unlock($1)`, lockID)
				conn.Release()
			}
			return release, nil
		}
		select {
		case <-ctx.Done():
			conn.Release()
			return nil, ctx.Err()
		case <-time.After(retry):
		}
	}
}
