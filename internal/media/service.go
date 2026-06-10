package media

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	mediav1 "github.com/fonvacano/yaxter/gen/yaxter/events/media/v1"
	"github.com/fonvacano/yaxter/internal/events"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

var (
	ErrBadType      = errors.New("media: unsupported content type")
	ErrTooLarge     = errors.New("media: size out of range")
	ErrNotFound     = errors.New("media: not found")
	ErrSizeMismatch = errors.New("media: uploaded size differs from declared")
)

const maxSizeBytes = 5 * 1024 * 1024 // assumption 5 in ARCHITECTURE.md

var allowedTypes = map[string]bool{
	"image/jpeg": true, "image/png": true, "image/webp": true,
}

type Ticket struct {
	MediaID   int64
	UploadURL string
	ExpiresAt time.Time
}

type Media struct {
	ID     int64
	Status string // pending | uploaded | ready | failed
}

type Service struct {
	db    *pgxpool.Pool
	store *Store
	ids   *snowflake.Generator
}

func NewService(db *pgxpool.Pool, store *Store, ids *snowflake.Generator) *Service {
	return &Service{db: db, store: store, ids: ids}
}

// Create validates, allocates the id, and returns the pre-signed PUT —
// uploads never transit the api pods (§2.5 step 1).
func (s *Service) Create(ctx context.Context, ownerID int64, contentType string, size int64) (Ticket, error) {
	if !allowedTypes[contentType] {
		return Ticket{}, ErrBadType
	}
	if size <= 0 || size > maxSizeBytes {
		return Ticket{}, ErrTooLarge
	}
	id := s.ids.Next()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO media (id, owner_id, content_type, size_bytes, status)
		VALUES ($1, $2, $3, $4, 'pending')`,
		id, ownerID, contentType, size); err != nil {
		return Ticket{}, err
	}
	url, err := s.store.PresignPut(ctx, uploadKey(id), contentType)
	if err != nil {
		return Ticket{}, err
	}
	return Ticket{MediaID: id, UploadURL: url, ExpiresAt: time.Now().Add(5 * time.Minute)}, nil
}

// Complete verifies the object landed (HEAD + declared-size check) and emits
// MediaUploaded in the same tx as the state flip (§2.5 step 2).
func (s *Service) Complete(ctx context.Context, ownerID, mediaID int64) (Media, error) {
	var contentType string
	var declared int64
	var status string
	err := s.db.QueryRow(ctx, `
		SELECT content_type, size_bytes, status FROM media
		WHERE id = $1 AND owner_id = $2`, mediaID, ownerID).
		Scan(&contentType, &declared, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Media{}, ErrNotFound
	}
	if err != nil {
		return Media{}, err
	}
	if status != "pending" { // idempotent re-complete
		return Media{ID: mediaID, Status: status}, nil
	}

	actual, err := s.store.Head(ctx, uploadKey(mediaID))
	if err != nil {
		return Media{}, err // ErrNoObject -> handler 409
	}
	if actual != declared {
		return Media{}, ErrSizeMismatch
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Media{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx,
		`UPDATE media SET status = 'uploaded' WHERE id = $1`, mediaID); err != nil {
		return Media{}, err
	}
	eventID := s.ids.Next()
	ev := &mediav1.MediaEvent{
		Envelope: events.NewEnvelope(ctx, eventID),
		Payload: &mediav1.MediaEvent_Uploaded{Uploaded: &mediav1.MediaUploaded{
			MediaId: mediaID, OwnerId: ownerID,
			ContentType: contentType, SizeBytes: declared,
		}},
	}
	if err := events.Emit(ctx, tx, eventID, "media.v1", events.Key(mediaID), ev); err != nil {
		return Media{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Media{}, err
	}
	return Media{ID: mediaID, Status: "uploaded"}, nil
}

func (s *Service) Get(ctx context.Context, ownerID, mediaID int64) (Media, error) {
	var m Media
	m.ID = mediaID
	err := s.db.QueryRow(ctx, `
		SELECT status FROM media WHERE id = $1 AND owner_id = $2`,
		mediaID, ownerID).Scan(&m.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Media{}, ErrNotFound
	}
	return m, err
}
