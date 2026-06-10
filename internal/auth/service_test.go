package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/fonvacano/yaxter/pkg/snowflake"
)

func newService(t *testing.T, pool *pgxpool.Pool) *Service {
	t.Helper()
	gen, err := snowflake.New(2)
	require.NoError(t, err)
	seed := make([]byte, ed25519.SeedSize)
	_, err = rand.Read(seed)
	require.NoError(t, err)
	iss, err := NewTokenIssuer("test-1", seed, 15*time.Minute)
	require.NoError(t, err)
	return NewService(pool, gen, iss, NewRefreshStore(pool, gen, 30*24*time.Hour))
}

func TestRegisterLoginLifecycle(t *testing.T) {
	pool := authTestPool(t)
	svc := newService(t, pool)
	ctx := context.Background()

	u, pair, err := svc.Register(ctx, "alice", "alice@example.com", "password123")
	require.NoError(t, err)
	require.Equal(t, "alice", u.Username)
	require.NotEmpty(t, pair.Access)
	require.NotEmpty(t, pair.Refresh)

	// Access token verifies to the new user's id.
	uid, err := svc.Issuer().Verify(pair.Access)
	require.NoError(t, err)
	require.Equal(t, u.ID, uid)

	// Duplicate username → ErrConflict.
	_, _, err = svc.Register(ctx, "alice", "other@example.com", "password123")
	require.ErrorIs(t, err, ErrConflict)

	// Login by username and by email.
	_, _, err = svc.Login(ctx, "alice", "password123")
	require.NoError(t, err)
	_, _, err = svc.Login(ctx, "alice@example.com", "password123")
	require.NoError(t, err)

	// Wrong password → uniform ErrInvalidCredentials.
	_, _, err = svc.Login(ctx, "alice", "nope")
	require.ErrorIs(t, err, ErrInvalidCredentials)
	// Unknown user → same error (no enumeration).
	_, _, err = svc.Login(ctx, "nobody", "password123")
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestRefreshAndLogoutFlow(t *testing.T) {
	pool := authTestPool(t)
	svc := newService(t, pool)
	ctx := context.Background()

	_, pair, err := svc.Register(ctx, "bob", "bob@example.com", "password123")
	require.NoError(t, err)

	pair2, err := svc.Refresh(ctx, pair.Refresh)
	require.NoError(t, err)
	require.NotEqual(t, pair.Refresh, pair2.Refresh)

	// Reuse of the rotated token kills the family.
	_, err = svc.Refresh(ctx, pair.Refresh)
	require.ErrorIs(t, err, ErrReused)
	_, err = svc.Refresh(ctx, pair2.Refresh)
	require.ErrorIs(t, err, ErrInvalidRefresh)

	// Fresh login then logout revokes.
	_, pair3, err := svc.Login(ctx, "bob", "password123")
	require.NoError(t, err)
	require.NoError(t, svc.Logout(ctx, pair3.Refresh))
	_, err = svc.Refresh(ctx, pair3.Refresh)
	require.ErrorIs(t, err, ErrInvalidRefresh)
}

func TestTokenPairForAndUserInfo(t *testing.T) {
	pool := authTestPool(t)
	svc := newService(t, pool)
	ctx := context.Background()

	u, _, err := svc.Register(ctx, "erin", "erin@example.com", "password123")
	require.NoError(t, err)

	pair, err := svc.TokenPairFor(ctx, u.ID)
	require.NoError(t, err)
	uid, err := svc.Issuer().Verify(pair.Access)
	require.NoError(t, err)
	require.Equal(t, u.ID, uid)

	info, providers, err := svc.UserInfo(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, "erin", info.Username)
	require.True(t, info.HasPassword)
	require.Empty(t, providers)

	_, err = pool.Exec(ctx, `
		INSERT INTO identities (user_id, provider, provider_user_id)
		VALUES ($1, 'yandex', 'y-1')`, u.ID)
	require.NoError(t, err)
	_, providers, err = svc.UserInfo(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"yandex"}, providers)
}

func TestLoginRejectsOAuthOnlyAccountUniformly(t *testing.T) {
	pool := authTestPool(t)
	svc := newService(t, pool)
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, pass_hash)
		VALUES (999, 'oauthonly', 'o@example.com', NULL)`)
	require.NoError(t, err)

	_, _, err = svc.Login(ctx, "oauthonly", "whatever")
	require.ErrorIs(t, err, ErrInvalidCredentials,
		"pass_hash NULL accounts get the same uniform error")
}
