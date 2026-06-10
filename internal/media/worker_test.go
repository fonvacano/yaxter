package media

import (
	"bytes"
	"context"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/image/webp"
)

func TestProcessUploadedHappyPath(t *testing.T) {
	svc, pool := testService(t)
	ctx := context.Background()

	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, testImage(800, 600)))
	body := buf.Bytes()

	ticket, err := svc.Create(ctx, 1, "image/png", int64(len(body)))
	require.NoError(t, err)
	uploadVia(t, ticket, body, "image/png")
	_, err = svc.Complete(ctx, 1, ticket.MediaID)
	require.NoError(t, err)

	require.NoError(t, svc.ProcessUploaded(ctx, ticket.MediaID))

	m, err := svc.Get(ctx, 1, ticket.MediaID)
	require.NoError(t, err)
	require.Equal(t, "ready", m.Status)

	for _, variant := range []string{"thumb", "feed", "orig"} {
		data, err := svc.store.Get(ctx, variantKey(variant, ticket.MediaID))
		require.NoError(t, err, variant)
		_, err = webp.Decode(bytes.NewReader(data))
		require.NoError(t, err, "%s variant must be valid webp", variant)
	}
	// Idempotent on redelivery.
	require.NoError(t, svc.ProcessUploaded(ctx, ticket.MediaID))
	_ = pool
}

func TestProcessUploadedNonImageGoesFailed(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()

	body := []byte("definitely not an image")
	ticket, err := svc.Create(ctx, 1, "image/png", int64(len(body)))
	require.NoError(t, err)
	uploadVia(t, ticket, body, "image/png")
	_, err = svc.Complete(ctx, 1, ticket.MediaID)
	require.NoError(t, err)

	require.NoError(t, svc.ProcessUploaded(ctx, ticket.MediaID),
		"a bad image is handled, not retried forever")
	m, err := svc.Get(ctx, 1, ticket.MediaID)
	require.NoError(t, err)
	require.Equal(t, "failed", m.Status)
}
