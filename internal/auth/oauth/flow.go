package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/fonvacano/yaxter/pkg/snowflake"
)

var (
	ErrUnknownProvider = errors.New("oauth: provider not enabled")
	ErrInvalidState    = errors.New("oauth: invalid, expired, or replayed state")
	// ErrLinkRequired: provider email matches an account but is not asserted
	// verified — the user must log in and link explicitly (§2.8 rule 2).
	ErrLinkRequired   = errors.New("oauth: explicit login-then-link required")
	ErrIdentityTaken  = errors.New("oauth: identity already linked to another account")
	ErrLastCredential = errors.New("oauth: cannot unlink the only credential")
)

const (
	ModeLogin = "login"
	ModeLink  = "link"
)

type stateData struct {
	Provider   string `json:"provider"`
	Verifier   string `json:"verifier"`
	RedirectTo string `json:"redirect_to"`
	Mode       string `json:"mode"`
	UserID     int64  `json:"user_id,omitempty"`
}

type Flow struct {
	db           *pgxpool.Pool
	rdb          *redis.Client
	ids          *snowflake.Generator
	providers    map[string]Provider
	redirectBase string // e.g. https://app.example.com
}

func NewFlow(db *pgxpool.Pool, rdb *redis.Client, ids *snowflake.Generator,
	providers map[string]Provider, redirectBase string) *Flow {
	return &Flow{db: db, rdb: rdb, ids: ids, providers: providers, redirectBase: redirectBase}
}

// Providers lists enabled providers for the discovery endpoint.
func (f *Flow) Providers() map[string]Provider { return f.providers }

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (f *Flow) callbackURI(provider string) string {
	return f.redirectBase + "/v1/auth/oauth/" + provider + "/callback"
}

// Start mints state + PKCE, stores them single-use in oas:{state} (TTL 5m,
// §2.3), and returns the provider URL to redirect to.
func (f *Flow) Start(ctx context.Context, provider, redirectTo, mode string, userID int64) (string, error) {
	p, ok := f.providers[provider]
	if !ok {
		return "", ErrUnknownProvider
	}
	state, err := randomToken()
	if err != nil {
		return "", err
	}
	verifier, err := randomToken()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	raw, err := json.Marshal(stateData{
		Provider: provider, Verifier: verifier,
		RedirectTo: redirectTo, Mode: mode, UserID: userID,
	})
	if err != nil {
		return "", err
	}
	if err := f.rdb.Set(ctx, "oas:"+state, raw, 5*time.Minute).Err(); err != nil {
		return "", err
	}
	return p.AuthCodeURL(state, challenge, f.callbackURI(provider)), nil
}

// Callback consumes the state (GETDEL — single use), exchanges the code, and
// applies the §2.8 linking rules. Returns the resolved local user id.
func (f *Flow) Callback(ctx context.Context, provider, code, state string) (int64, error) {
	p, ok := f.providers[provider]
	if !ok {
		return 0, ErrUnknownProvider
	}
	raw, err := f.rdb.GetDel(ctx, "oas:"+state).Bytes()
	if errors.Is(err, redis.Nil) {
		return 0, ErrInvalidState
	}
	if err != nil {
		return 0, err
	}
	var data stateData
	if err := json.Unmarshal(raw, &data); err != nil || data.Provider != provider {
		return 0, ErrInvalidState
	}

	tok, err := p.Exchange(ctx, code, data.Verifier, f.callbackURI(provider))
	if err != nil {
		return 0, fmt.Errorf("oauth: exchange: %w", err)
	}
	ident, err := p.Identity(ctx, tok)
	if err != nil {
		return 0, fmt.Errorf("oauth: identity: %w", err)
	}

	if data.Mode == ModeLink {
		return data.UserID, f.link(ctx, data.UserID, provider, ident)
	}

	// Rule 1: known identity -> that user.
	var userID int64
	err = f.db.QueryRow(ctx, `
		SELECT user_id FROM global_identities
		WHERE provider = $1 AND provider_user_id = $2`,
		provider, ident.ProviderUserID).Scan(&userID)
	if err == nil {
		return userID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}

	// Rule 2: email matches an existing account.
	err = f.db.QueryRow(ctx,
		`SELECT id FROM users WHERE email = $1`, ident.Email).Scan(&userID)
	if err == nil {
		if !ident.EmailVerified {
			return 0, ErrLinkRequired
		}
		return userID, f.link(ctx, userID, provider, ident)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}

	// Rule 3: brand-new user.
	return f.createUser(ctx, provider, ident)
}

var usernameSanitizer = regexp.MustCompile(`[^A-Za-z0-9_]`)

func (f *Flow) createUser(ctx context.Context, provider string, ident Identity) (int64, error) {
	base := usernameSanitizer.ReplaceAllString(ident.Login, "_")
	if len(base) < 3 {
		base = "user"
	}
	if len(base) > 24 {
		base = base[:24]
	}
	tx, err := f.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	userID := f.ids.Next()
	username := base
	for i := 1; ; i++ {
		var taken bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM users WHERE username = $1)`, username).
			Scan(&taken); err != nil {
			return 0, err
		}
		if !taken {
			break
		}
		username = fmt.Sprintf("%s_%d", base, i)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, email, pass_hash)
		VALUES ($1, $2, $3, NULL)`, userID, username, strings.ToLower(ident.Email)); err != nil {
		return 0, err
	}
	if err := f.insertIdentity(ctx, tx, userID, provider, ident); err != nil {
		return 0, err
	}
	return userID, tx.Commit(ctx)
}

// link attaches the identity to userID with audit log + in-app notification
// (plan deviation #1: the notification is a direct row in the same tx).
func (f *Flow) link(ctx context.Context, userID int64, provider string, ident Identity) error {
	var existing int64
	err := f.db.QueryRow(ctx, `
		SELECT user_id FROM global_identities
		WHERE provider = $1 AND provider_user_id = $2`,
		provider, ident.ProviderUserID).Scan(&existing)
	if err == nil {
		if existing == userID {
			return nil // already linked: idempotent
		}
		return ErrIdentityTaken
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	tx, err := f.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := f.insertIdentity(ctx, tx, userID, provider, ident); err != nil {
		return err
	}
	detail, _ := json.Marshal(map[string]string{
		"provider": provider, "provider_user_id": ident.ProviderUserID,
	})
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (id, user_id, action, detail)
		VALUES ($1, $2, 'oauth_link', $3)`, f.ids.Next(), userID, detail); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO notifications (id, user_id, kind, actor_id)
		VALUES ($1, $2, 'oauth_link', $2)`, f.ids.Next(), userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (f *Flow) insertIdentity(ctx context.Context, tx pgx.Tx, userID int64, provider string, ident Identity) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO identities (user_id, provider, provider_user_id, email)
		VALUES ($1, $2, $3, $4)`,
		userID, provider, ident.ProviderUserID, strings.ToLower(ident.Email)); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO global_identities (provider, provider_user_id, user_id)
		VALUES ($1, $2, $3)`, provider, ident.ProviderUserID, userID)
	return err
}

// Unlink removes the provider unless it is the account's only credential.
func (f *Flow) Unlink(ctx context.Context, userID int64, provider string) error {
	var hasPassword bool
	var identityCount int
	err := f.db.QueryRow(ctx, `
		SELECT u.pass_hash IS NOT NULL,
		       (SELECT count(*) FROM identities i WHERE i.user_id = u.id)
		FROM users u WHERE u.id = $1`, userID).Scan(&hasPassword, &identityCount)
	if err != nil {
		return err
	}
	if !hasPassword && identityCount <= 1 {
		return ErrLastCredential
	}
	tx, err := f.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	tag, err := tx.Exec(ctx, `
		DELETE FROM identities WHERE user_id = $1 AND provider = $2`, userID, provider)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM global_identities WHERE provider = $1 AND user_id = $2`,
		provider, userID); err != nil {
		return err
	}
	detail, _ := json.Marshal(map[string]string{"provider": provider})
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (id, user_id, action, detail)
		VALUES ($1, $2, 'oauth_unlink', $3)`, f.ids.Next(), userID, detail); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
