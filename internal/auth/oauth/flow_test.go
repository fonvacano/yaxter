package oauth

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

type fakeProvider struct{ ident Identity }

func (f *fakeProvider) Name() string        { return "fake" }
func (f *fakeProvider) DisplayName() string { return "Fake" }
func (f *fakeProvider) AuthCodeURL(state, challenge, redirect string) string {
	return "https://fake/auth?state=" + state
}
func (f *fakeProvider) Exchange(context.Context, string, string, string) (*Token, error) {
	return &Token{AccessToken: "at"}, nil
}
func (f *fakeProvider) Identity(context.Context, *Token) (Identity, error) {
	return f.ident, nil
}

func testFlow(t *testing.T, ident Identity) (*Flow, *pgxpool.Pool, *miniredis.Miniredis) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: requires Docker")
	}
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("yaxter"), tcpostgres.WithUsername("yaxter"),
		tcpostgres.WithPassword("yaxter"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	m, err := migrate.New("file://../../../migrations", dsn)
	require.NoError(t, err)
	require.NoError(t, m.Up())
	m.Close()
	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	gen, err := snowflake.New(7)
	require.NoError(t, err)
	f := NewFlow(pool, rdb, gen, map[string]Provider{"fake": &fakeProvider{ident: ident}},
		"http://localhost:8080")
	return f, pool, mr
}

func startAndState(t *testing.T, f *Flow, mode string, userID int64) string {
	t.Helper()
	authURL, err := f.Start(context.Background(), "fake", "/", mode, userID)
	require.NoError(t, err)
	u, err := url.Parse(authURL)
	require.NoError(t, err)
	state := u.Query().Get("state")
	require.NotEmpty(t, state)
	return state
}

func TestNewUserSignupWithUsernameCollision(t *testing.T) {
	f, pool, _ := testFlow(t, Identity{
		ProviderUserID: "p-1", Email: "new@example.com", EmailVerified: true, Login: "taken",
	})
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, pass_hash)
		VALUES (1, 'taken', 'other@example.com', 'x')`)
	require.NoError(t, err)

	state := startAndState(t, f, ModeLogin, 0)
	userID, err := f.Callback(ctx, "fake", "code", state)
	require.NoError(t, err)
	require.NotEqualValues(t, 1, userID)

	var username string
	var passHash *string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT username, pass_hash FROM users WHERE id = $1`, userID).
		Scan(&username, &passHash))
	require.Equal(t, "taken_1", username, "collision gets a suffix (§2.8 rule 3)")
	require.Nil(t, passHash, "oauth-born accounts have no password")

	// Repeat login resolves to the same user via the identity (§2.8 rule 1).
	state2 := startAndState(t, f, ModeLogin, 0)
	again, err := f.Callback(ctx, "fake", "code", state2)
	require.NoError(t, err)
	require.Equal(t, userID, again)
}

func TestVerifiedEmailAutoLinksWithAuditAndNotification(t *testing.T) {
	f, pool, _ := testFlow(t, Identity{
		ProviderUserID: "p-2", Email: "carol@example.com", EmailVerified: true, Login: "carol",
	})
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, pass_hash)
		VALUES (5, 'carol', 'carol@example.com', 'x')`)
	require.NoError(t, err)

	state := startAndState(t, f, ModeLogin, 0)
	userID, err := f.Callback(ctx, "fake", "code", state)
	require.NoError(t, err)
	require.EqualValues(t, 5, userID, "verified email auto-links to the existing user")

	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE user_id=5 AND action='oauth_link'`).Scan(&n))
	require.Equal(t, 1, n, "auto-link is audit-logged (§2.8 rule 2)")
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE user_id=5 AND kind='oauth_link'`).Scan(&n))
	require.Equal(t, 1, n, "user is notified about the link")
}

func TestUnverifiedEmailRequiresExplicitLink(t *testing.T) {
	f, pool, _ := testFlow(t, Identity{
		ProviderUserID: "p-3", Email: "dan@example.com", EmailVerified: false, Login: "dan",
	})
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, pass_hash)
		VALUES (6, 'dan', 'dan@example.com', 'x')`)
	require.NoError(t, err)

	state := startAndState(t, f, ModeLogin, 0)
	_, err = f.Callback(ctx, "fake", "code", state)
	require.ErrorIs(t, err, ErrLinkRequired, "no silent auto-link on unverified email")
}

func TestStateIsSingleUseAndExpires(t *testing.T) {
	f, _, mr := testFlow(t, Identity{ProviderUserID: "p-4", Email: "e@x.c", EmailVerified: true, Login: "e"})
	ctx := context.Background()

	state := startAndState(t, f, ModeLogin, 0)
	_, err := f.Callback(ctx, "fake", "code", state)
	require.NoError(t, err)
	_, err = f.Callback(ctx, "fake", "code", state)
	require.ErrorIs(t, err, ErrInvalidState, "replayed state must be rejected")

	state2 := startAndState(t, f, ModeLogin, 0)
	mr.FastForward(6 * time.Minute) // oas: TTL is 5m (§2.3)
	_, err = f.Callback(ctx, "fake", "code", state2)
	require.ErrorIs(t, err, ErrInvalidState, "expired state must be rejected")
}

func TestExplicitLinkAndUnlinkGuards(t *testing.T) {
	f, pool, _ := testFlow(t, Identity{
		ProviderUserID: "p-5", Email: "f@x.c", EmailVerified: false, Login: "frank",
	})
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, pass_hash) VALUES
		(7, 'frank', 'f@x.c', 'x'), (8, 'grace', 'g@x.c', NULL)`)
	require.NoError(t, err)

	// Explicit link in mode=link attaches to the session user despite the
	// unverified email (the user proved ownership by logging in).
	state := startAndState(t, f, ModeLink, 7)
	userID, err := f.Callback(ctx, "fake", "code", state)
	require.NoError(t, err)
	require.EqualValues(t, 7, userID)

	// The same identity cannot be linked to another account.
	state2 := startAndState(t, f, ModeLink, 8)
	_, err = f.Callback(ctx, "fake", "code", state2)
	require.ErrorIs(t, err, ErrIdentityTaken)

	// Unlink works for a passworded account...
	require.NoError(t, f.Unlink(ctx, 7, "fake"))
	// ...but an OAuth-only account cannot drop its last credential.
	// (pgx forbids multi-statement Exec — one call each.)
	_, err = pool.Exec(ctx, `
		INSERT INTO identities (user_id, provider, provider_user_id) VALUES (8, 'fake', 'p-8')`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO global_identities (provider, provider_user_id, user_id) VALUES ('fake', 'p-8', 8)`)
	require.NoError(t, err)
	require.ErrorIs(t, f.Unlink(ctx, 8, "fake"), ErrLastCredential)
}
