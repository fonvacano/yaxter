// Package pgx constructs PostgreSQL connection pools. Shard routing comes
// later (T0.4); this is the single-pool constructor used by everything.
package pgx

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool parses dsn, connects, and verifies the connection with a ping.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
