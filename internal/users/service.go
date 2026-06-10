// Package users implements profiles and the follow graph (ARCHITECTURE.md §2.2).
package users

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/fonvacano/yaxter/pkg/snowflake"
)

var (
	ErrNotFound      = errors.New("users: not found")
	ErrSelfFollow    = errors.New("users: cannot follow yourself")
	ErrMediaNotReady = errors.New("users: avatar media not ready")
)

type User struct {
	ID             int64
	Username       string
	Email          string
	Bio            string
	AvatarKey      *string
	FollowersCount int
	FollowingCount int
	HasPassword    bool
	CreatedAt      time.Time
}

type UpdateProfile struct {
	Bio           *string
	AvatarMediaID *int64
}

type Service struct {
	db                 *pgxpool.Pool
	rdb                *redis.Client
	ids                *snowflake.Generator
	celebrityThreshold int
}

func NewService(db *pgxpool.Pool, rdb *redis.Client, ids *snowflake.Generator, celebrityThreshold int) *Service {
	return &Service{db: db, rdb: rdb, ids: ids, celebrityThreshold: celebrityThreshold}
}

const userColumns = `id, username, email, bio, avatar_key,
	followers_count, following_count, pass_hash IS NOT NULL, created_at`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.Bio, &u.AvatarKey,
		&u.FollowersCount, &u.FollowingCount, &u.HasPassword, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Service) GetByID(ctx context.Context, id int64) (User, error) {
	return scanUser(s.db.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}

func (s *Service) GetByUsername(ctx context.Context, username string) (User, error) {
	return scanUser(s.db.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE username = $1`, username))
}

func (s *Service) UpdateProfile(ctx context.Context, userID int64, up UpdateProfile) (User, error) {
	if up.AvatarMediaID != nil {
		var ok bool
		err := s.db.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM media
				WHERE id = $1 AND owner_id = $2 AND status = 'ready'
			)`, *up.AvatarMediaID, userID).Scan(&ok)
		if err != nil {
			return User{}, err
		}
		if !ok {
			return User{}, ErrMediaNotReady
		}
	}
	var avatarKey *string
	if up.AvatarMediaID != nil {
		k := fmt.Sprintf("%d", *up.AvatarMediaID)
		avatarKey = &k
	}
	u, err := scanUser(s.db.QueryRow(ctx, `
		UPDATE users SET
			bio        = COALESCE($2, bio),
			avatar_key = COALESCE($3, avatar_key)
		WHERE id = $1
		RETURNING `+userColumns, userID, up.Bio, avatarKey))
	if err != nil {
		return User{}, err
	}
	s.rdb.Del(ctx, fmt.Sprintf("usr:%d", userID))
	return u, nil
}
