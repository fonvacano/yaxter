package auth

import (
	"context"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

func authTestPool(t *testing.T) *pgxpool.Pool {
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
	m, err := migrate.New("file://../../migrations", dsn)
	require.NoError(t, err)
	require.NoError(t, m.Up())
	_, _ = m.Close()
	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func newStore(t *testing.T, pool *pgxpool.Pool) *RefreshStore {
	t.Helper()
	gen, err := snowflake.New(1)
	require.NoError(t, err)
	return NewRefreshStore(pool, gen, 30*24*time.Hour)
}

func TestIssueAndRotate(t *testing.T) {
	pool := authTestPool(t)
	s := newStore(t, pool)
	ctx := context.Background()

	tok1, err := s.IssueNewFamily(ctx, 42)
	require.NoError(t, err)

	uid, tok2, err := s.Rotate(ctx, tok1)
	require.NoError(t, err)
	require.EqualValues(t, 42, uid)
	require.NotEqual(t, tok1, tok2)

	// The rotated-to token works.
	_, tok3, err := s.Rotate(ctx, tok2)
	require.NoError(t, err)
	require.NotEmpty(t, tok3)
}

func TestReuseRevokesWholeFamily(t *testing.T) {
	pool := authTestPool(t)
	s := newStore(t, pool)
	ctx := context.Background()

	tok1, err := s.IssueNewFamily(ctx, 42)
	require.NoError(t, err)
	_, tok2, err := s.Rotate(ctx, tok1)
	require.NoError(t, err)

	// Replay of the already-rotated tok1 = theft signal.
	_, _, err = s.Rotate(ctx, tok1)
	require.ErrorIs(t, err, ErrReused)

	// The whole family is dead, including the newest token.
	_, _, err = s.Rotate(ctx, tok2)
	require.ErrorIs(t, err, ErrInvalidRefresh)
}

func TestUnknownAndRevokedTokens(t *testing.T) {
	pool := authTestPool(t)
	s := newStore(t, pool)
	ctx := context.Background()

	_, _, err := s.Rotate(ctx, "no-such-token")
	require.ErrorIs(t, err, ErrInvalidRefresh)

	tok, err := s.IssueNewFamily(ctx, 7)
	require.NoError(t, err)
	require.NoError(t, s.RevokeFamilyByToken(ctx, tok)) // logout
	_, _, err = s.Rotate(ctx, tok)
	require.ErrorIs(t, err, ErrInvalidRefresh)
}
