package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fonvacano/yaxter/pkg/snowflake"
)

var (
	// ErrInvalidRefresh covers unknown, expired, and family-revoked tokens —
	// deliberately indistinguishable to callers (uniform errors, §7).
	ErrInvalidRefresh = errors.New("auth: invalid refresh token")
	// ErrReused signals rotation replay: the family has been revoked.
	ErrReused = errors.New("auth: refresh token reuse detected")
)

// RefreshStore implements §2.8 rotating refresh tokens: opaque 256-bit
// values, only their SHA-256 stored, rotated on every use; replaying a
// rotated token revokes its whole family.
type RefreshStore struct {
	pool *pgxpool.Pool
	ids  *snowflake.Generator
	ttl  time.Duration
}

func NewRefreshStore(pool *pgxpool.Pool, ids *snowflake.Generator, ttl time.Duration) *RefreshStore {
	return &RefreshStore{pool: pool, ids: ids, ttl: ttl}
}

func newOpaqueToken() (token, hash string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, hashToken(token), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawStdEncoding.EncodeToString(sum[:])
}

// IssueNewFamily mints the first token of a new family (login/register).
func (s *RefreshStore) IssueNewFamily(ctx context.Context, userID int64) (string, error) {
	id := s.ids.Next()
	return s.insert(ctx, id, userID, id) // family_id = first token's id
}

func (s *RefreshStore) insert(ctx context.Context, id, userID, familyID int64) (string, error) {
	token, hash, err := newOpaqueToken()
	if err != nil {
		return "", err
	}
	// make_interval, not Duration.String()::interval — Postgres parses the
	// "m" in Go's "720h0m0s" as months.
	_, err = s.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (id, user_id, family_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4, now() + make_interval(secs => $5))`,
		id, userID, familyID, hash, s.ttl.Seconds())
	return token, err
}

// Rotate validates token, revokes it, and issues a successor in the same
// family. Replay of an already-revoked token revokes the family (ErrReused).
func (s *RefreshStore) Rotate(ctx context.Context, token string) (int64, string, error) {
	var id, userID, familyID int64
	var expiresAt time.Time
	var revokedAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, family_id, expires_at, revoked_at
		FROM refresh_tokens WHERE token_hash = $1`, hashToken(token)).
		Scan(&id, &userID, &familyID, &expiresAt, &revokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", ErrInvalidRefresh
	}
	if err != nil {
		return 0, "", err
	}
	if revokedAt != nil {
		// A successor in the family means this token was directly rotated and
		// is now being replayed — genuine theft signal. Tokens revoked as
		// collateral by a prior revokeFamily call have no successor.
		var hasSuccessor bool
		if err := s.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM refresh_tokens WHERE family_id = $1 AND id > $2)`,
			familyID, id).Scan(&hasSuccessor); err != nil {
			return 0, "", err
		}
		if hasSuccessor {
			if err := s.revokeFamily(ctx, familyID); err != nil {
				return 0, "", err
			}
			return 0, "", ErrReused
		}
		return 0, "", ErrInvalidRefresh
	}
	if time.Now().After(expiresAt) {
		return 0, "", ErrInvalidRefresh
	}
	// Family-wide revocations between the SELECT above and here are caught
	// by the revoked_at guard on this UPDATE.
	tag, err := s.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE id = $1 AND revoked_at IS NULL`, id)
	if err != nil {
		return 0, "", err
	}
	if tag.RowsAffected() == 0 {
		return 0, "", ErrInvalidRefresh
	}
	next, err := s.insert(ctx, s.ids.Next(), userID, familyID)
	if err != nil {
		return 0, "", err
	}
	return userID, next, nil
}

// RevokeFamilyByToken revokes the token's whole family (logout).
func (s *RefreshStore) RevokeFamilyByToken(ctx context.Context, token string) error {
	var familyID int64
	err := s.pool.QueryRow(ctx,
		`SELECT family_id FROM refresh_tokens WHERE token_hash = $1`,
		hashToken(token)).Scan(&familyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // logout with unknown token is a no-op, not an oracle
	}
	if err != nil {
		return err
	}
	return s.revokeFamily(ctx, familyID)
}

func (s *RefreshStore) revokeFamily(ctx context.Context, familyID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now()
		WHERE family_id = $1 AND revoked_at IS NULL`, familyID)
	return err
}
