package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fonvacano/yaxter/pkg/snowflake"
)

var (
	// ErrInvalidCredentials is uniform across unknown user, wrong password,
	// and OAuth-only accounts — no user enumeration (§7).
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrConflict           = errors.New("auth: username or email already taken")
)

type User struct {
	ID          int64
	Username    string
	Email       string
	Bio         string
	CreatedAt   time.Time
	HasPassword bool
}

type TokenPair struct {
	Access    string
	Refresh   string
	ExpiresIn int
}

// Service owns the password-credential lifecycle. It queries the global
// pool (username/email are globally unique there, §2.2); per-user-shard
// routing applies to the user-keyed domain modules, not credential lookup.
type Service struct {
	db      *pgxpool.Pool
	ids     *snowflake.Generator
	issuer  *TokenIssuer
	refresh *RefreshStore
}

func NewService(db *pgxpool.Pool, ids *snowflake.Generator, issuer *TokenIssuer, refresh *RefreshStore) *Service {
	return &Service{db: db, ids: ids, issuer: issuer, refresh: refresh}
}

func (s *Service) Issuer() *TokenIssuer { return s.issuer }

func (s *Service) pair(ctx context.Context, userID int64) (TokenPair, error) {
	access, err := s.issuer.Issue(userID)
	if err != nil {
		return TokenPair{}, err
	}
	refresh, err := s.refresh.IssueNewFamily(ctx, userID)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{Access: access, Refresh: refresh, ExpiresIn: s.issuer.TTLSeconds()}, nil
}

func (s *Service) Register(ctx context.Context, username, email, password string) (User, TokenPair, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, TokenPair{}, err
	}
	u := User{ID: s.ids.Next(), Username: username, Email: email}
	err = s.db.QueryRow(ctx, `
		INSERT INTO users (id, username, email, pass_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at`,
		u.ID, username, email, hash).Scan(&u.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
		return User{}, TokenPair{}, ErrConflict
	}
	if err != nil {
		return User{}, TokenPair{}, err
	}
	pair, err := s.pair(ctx, u.ID)
	return u, pair, err
}

func (s *Service) Login(ctx context.Context, login, password string) (User, TokenPair, error) {
	var u User
	var passHash *string
	err := s.db.QueryRow(ctx, `
		SELECT id, username, email, bio, created_at, pass_hash
		FROM users WHERE username = $1 OR email = $1`, login).
		Scan(&u.ID, &u.Username, &u.Email, &u.Bio, &u.CreatedAt, &passHash)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && passHash == nil) {
		// Burn comparable time for unknown users / OAuth-only accounts so
		// timing doesn't leak existence; then return the uniform error.
		_, _ = VerifyPassword(password,
			"$argon2id$v=19$m=65536,t=1,p=4$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
		return User{}, TokenPair{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, TokenPair{}, err
	}
	ok, err := VerifyPassword(password, *passHash)
	if err != nil || !ok {
		return User{}, TokenPair{}, ErrInvalidCredentials
	}
	pair, err := s.pair(ctx, u.ID)
	return u, pair, err
}

func (s *Service) Refresh(ctx context.Context, token string) (TokenPair, error) {
	userID, next, err := s.refresh.Rotate(ctx, token)
	if err != nil {
		return TokenPair{}, err
	}
	access, err := s.issuer.Issue(userID)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{Access: access, Refresh: next, ExpiresIn: s.issuer.TTLSeconds()}, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.refresh.RevokeFamilyByToken(ctx, token)
}

// TokenPairFor issues a fresh pair for an externally-authenticated user —
// OAuth reuses (never reimplements) the session machinery (§2.8 / T1.6).
func (s *Service) TokenPairFor(ctx context.Context, userID int64) (TokenPair, error) {
	return s.pair(ctx, userID)
}

// UserInfo loads the profile plus linked provider names (PrivateUser fields).
func (s *Service) UserInfo(ctx context.Context, userID int64) (User, []string, error) {
	var u User
	var hasPassword bool
	err := s.db.QueryRow(ctx, `
		SELECT id, username, email, bio, created_at, pass_hash IS NOT NULL
		FROM users WHERE id = $1`, userID).
		Scan(&u.ID, &u.Username, &u.Email, &u.Bio, &u.CreatedAt, &hasPassword)
	if err != nil {
		return User{}, nil, err
	}
	u.HasPassword = hasPassword
	rows, err := s.db.Query(ctx,
		`SELECT provider FROM identities WHERE user_id = $1 ORDER BY provider`, userID)
	if err != nil {
		return User{}, nil, err
	}
	defer rows.Close()
	providers := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return User{}, nil, err
		}
		providers = append(providers, p)
	}
	return u, providers, rows.Err()
}
