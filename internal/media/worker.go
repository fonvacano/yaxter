package media

import (
	"context"
	"errors"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	mediav1 "github.com/fonvacano/yaxter/gen/yaxter/events/media/v1"
)

// ProcessUploaded runs the §2.5 step-3 pipeline for one media id:
// download -> validate/re-encode -> upload variants -> ready.
// Idempotent: a redelivered event on a non-'uploaded' row is a no-op.
func (s *Service) ProcessUploaded(ctx context.Context, mediaID int64) error {
	var status string
	if err := s.db.QueryRow(ctx,
		`SELECT status FROM media WHERE id = $1`, mediaID).Scan(&status); err != nil {
		return err
	}
	if status != "uploaded" {
		return nil
	}

	raw, err := s.store.Get(ctx, uploadKey(mediaID))
	if err != nil {
		return err // transient storage error: redelivery retries
	}
	variants, err := ProcessImage(raw)
	if errors.Is(err, ErrNotImage) {
		_, uerr := s.db.Exec(ctx,
			`UPDATE media SET status = 'failed' WHERE id = $1`, mediaID)
		return uerr // permanent failure: park as failed, don't retry
	}
	if err != nil {
		return err
	}
	for name, data := range variants {
		if err := s.store.Put(ctx, variantKey(name, mediaID), data, "image/webp"); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(ctx,
		`UPDATE media SET status = 'ready', ready_at = now() WHERE id = $1`, mediaID)
	return err
}

// HandleRecord decodes a media.v1 record and processes it (worker + tests).
func (s *Service) HandleRecord(ctx context.Context, rec *kgo.Record) error {
	var ev mediav1.MediaEvent
	if err := proto.Unmarshal(rec.Value, &ev); err != nil {
		return err
	}
	if up := ev.GetUploaded(); up != nil {
		return s.ProcessUploaded(ctx, up.GetMediaId())
	}
	return nil
}
