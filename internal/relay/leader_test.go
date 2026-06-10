package relay

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
)

func pgPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("yaxter"),
		tcpostgres.WithUsername("yaxter"),
		tcpostgres.WithPassword("yaxter"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func TestLeadershipIsExclusive(t *testing.T) {
	pool := pgPool(t)
	ctx := context.Background()

	release1, err := AcquireLeadership(ctx, pool, 42, 50*time.Millisecond)
	require.NoError(t, err)

	// Second contender must NOT acquire while the first holds the lock.
	ctx2, cancel2 := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel2()
	_, err = AcquireLeadership(ctx2, pool, 42, 50*time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	// After release, acquisition succeeds.
	release1()
	release2, err := AcquireLeadership(ctx, pool, 42, 50*time.Millisecond)
	require.NoError(t, err)
	release2()
}

func TestDifferentLockIDsAreIndependent(t *testing.T) {
	pool := pgPool(t)
	ctx := context.Background()

	r1, err := AcquireLeadership(ctx, pool, 1, 50*time.Millisecond)
	require.NoError(t, err)
	defer r1()
	r2, err := AcquireLeadership(ctx, pool, 2, 50*time.Millisecond)
	require.NoError(t, err)
	defer r2()
}
