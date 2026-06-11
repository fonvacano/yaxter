// Package notifications implements in-app notifications: a worker that turns
// follow/engagement events into rows, and read endpoints. ARCHITECTURE.md §2.3.
package notifications

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Kinds match the OpenAPI NotificationKind enum and the migration CHECK set.
const (
	KindFollow  = "follow"
	KindLike    = "like"
	KindRetweet = "retweet"
)

// Notification is a row joined with its actor's display fields.
type Notification struct {
	ID          int64
	UserID      int64
	Kind        string
	ActorID     int64
	ActorName   string
	ActorAvatar *string
	SubjectID   *int64
	CreatedAt   time.Time
	ReadAt      *time.Time
}

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service { return &Service{db: db} }

// insert writes one notification row. subjectID may be nil (follow). Caller
// supplies the snowflake id (worker leases a node).
func insert(ctx context.Context, db *pgxpool.Pool, id, userID int64, kind string, actorID int64, subjectID *int64) error {
	_, err := db.Exec(ctx, `
		INSERT INTO notifications (id, user_id, kind, actor_id, subject_id, created_at)
		VALUES ($1, $2, $3, $4, $5, now())`, id, userID, kind, actorID, subjectID)
	return err
}

// List returns notifications for userID newest-first, cursor = id DESC.
// next is the cursor for the following page (nil on last page).
func (s *Service) List(ctx context.Context, userID, cursor int64, limit int) ([]Notification, *int64, error) {
	rows, err := s.db.Query(ctx, `
		SELECT n.id, n.user_id, n.kind, n.actor_id, n.subject_id, n.created_at, n.read_at,
		       u.username, u.avatar_key
		FROM notifications n
		JOIN users u ON u.id = n.actor_id
		WHERE n.user_id = $1 AND ($2 = 0 OR n.id < $2)
		ORDER BY n.id DESC
		LIMIT $3`, userID, cursor, limit+1)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	items := make([]Notification, 0, limit+1)
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Kind, &n.ActorID, &n.SubjectID,
			&n.CreatedAt, &n.ReadAt, &n.ActorName, &n.ActorAvatar); err != nil {
			return nil, nil, err
		}
		items = append(items, n)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var next *int64
	if len(items) > limit {
		c := items[limit-1].ID
		next = &c
		items = items[:limit]
	}
	return items, next, nil
}

func (s *Service) UnreadCount(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.db.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`,
		userID).Scan(&n)
	return n, err
}

// MarkRead marks all of userID's notifications with id <= upToID as read.
func (s *Service) MarkRead(ctx context.Context, userID, upToID int64) error {
	_, err := s.db.Exec(ctx,
		`UPDATE notifications SET read_at = now()
		 WHERE user_id = $1 AND id <= $2 AND read_at IS NULL`, userID, upToID)
	return err
}
