package media

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/protobuf/proto"

	mediav1 "github.com/fonvacano/yaxter/gen/yaxter/events/media/v1"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

func testService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	st := testStore(t) // skips under -short
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("yaxter"), tcpostgres.WithUsername("yaxter"),
		tcpostgres.WithPassword("yaxter"), tcpostgres.BasicWaitStrategies())
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	m, err := migrate.New("file://../../migrations", dsn)
	require.NoError(t, err)
	require.NoError(t, m.Up())
	_, _ = m.Close()
	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	gen, err := snowflake.New(6)
	require.NoError(t, err)
	return NewService(pool, st, gen), pool
}

func uploadVia(t *testing.T, ticket Ticket, body []byte, contentType string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, ticket.UploadURL, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", contentType)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
}

func TestCreateValidates(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, 1, "application/pdf", 100)
	require.ErrorIs(t, err, ErrBadType)
	_, err = svc.Create(ctx, 1, "image/png", 6*1024*1024)
	require.ErrorIs(t, err, ErrTooLarge)
	_, err = svc.Create(ctx, 1, "image/png", 0)
	require.ErrorIs(t, err, ErrTooLarge)
}

func TestCreateUploadCompleteFlow(t *testing.T) {
	svc, pool := testService(t)
	ctx := context.Background()

	body := []byte("fake image bytes")
	ticket, err := svc.Create(ctx, 1, "image/png", int64(len(body)))
	require.NoError(t, err)
	require.NotZero(t, ticket.MediaID)

	// Complete before upload: object missing -> ErrNoObject (handler: 409).
	_, err = svc.Complete(ctx, 1, ticket.MediaID)
	require.ErrorIs(t, err, ErrNoObject)

	uploadVia(t, ticket, body, "image/png")

	// Wrong owner gets not-found (no resource enumeration).
	_, err = svc.Complete(ctx, 2, ticket.MediaID)
	require.ErrorIs(t, err, ErrNotFound)

	m, err := svc.Complete(ctx, 1, ticket.MediaID)
	require.NoError(t, err)
	require.Equal(t, "uploaded", m.Status)

	var payload []byte
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT payload FROM outbox WHERE topic = 'media.v1'`).Scan(&payload))
	var ev mediav1.MediaEvent
	require.NoError(t, proto.Unmarshal(payload, &ev))
	require.Equal(t, ticket.MediaID, ev.GetUploaded().GetMediaId())

	got, err := svc.Get(ctx, 1, ticket.MediaID)
	require.NoError(t, err)
	require.Equal(t, "uploaded", got.Status)
}

func TestCompleteRejectsSizeMismatch(t *testing.T) {
	svc, _ := testService(t)
	ctx := context.Background()

	ticket, err := svc.Create(ctx, 1, "image/png", 100) // declared 100
	require.NoError(t, err)
	uploadVia(t, ticket, bytes.Repeat([]byte{1}, 50), "image/png") // actual 50

	_, err = svc.Complete(ctx, 1, ticket.MediaID)
	require.ErrorIs(t, err, ErrSizeMismatch,
		"deviation #2: size enforced at complete instead of presign policy")
}
