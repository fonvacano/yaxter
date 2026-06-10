package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/fonvacano/yaxter/internal/media"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

// liveHandlerWithS3 is like liveHandler but also spins a MinIO container and
// threads the media store through Deps so the media endpoints work end-to-end.
func liveHandlerWithS3(t *testing.T) http.Handler {
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
	m, err := migrate.New("file://../../migrations", dsn)
	require.NoError(t, err)
	require.NoError(t, m.Up())
	_, _ = m.Close()
	pool, err := pgxkit.NewPool(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	mc, err := tcminio.Run(ctx, "minio/minio:RELEASE.2024-11-07T00-52-20Z")
	require.NoError(t, err)
	t.Cleanup(func() { _ = mc.Terminate(context.Background()) })
	endpoint, err := mc.ConnectionString(ctx)
	require.NoError(t, err)
	store, err := media.NewStore(ctx, media.StoreConfig{
		Endpoint:     "http://" + endpoint,
		Region:       "us-east-1",
		AccessKey:    mc.Username,
		SecretKey:    mc.Password,
		Bucket:       "media",
		UsePathStyle: true,
	})
	require.NoError(t, err)
	require.NoError(t, store.EnsureBucket(ctx))

	mr := miniredis.RunT(t)
	gen, err := snowflake.New(5)
	require.NoError(t, err)
	h, err := NewHandler(Deps{
		DB:    pool,
		Redis: redis.NewClient(&redis.Options{Addr: mr.Addr()}),
		IDs:   gen, JWTKid: "test-1",
		JWTSeed:       bytes.Repeat([]byte{7}, 32),
		AuthRateLimit: 100,
		MediaBaseURL:  "https://media.example.test",
		MediaStore:    store,
	})
	require.NoError(t, err)
	return h
}

func TestMediaEndpointsLifecycle(t *testing.T) {
	h := liveHandlerWithS3(t)
	tok := registerAndToken(t, h, "uploader")
	hdrs := map[string]string{
		"Idempotency-Key": uuid.NewString(), "Authorization": "Bearer " + tok,
	}

	bad := postJSON(t, h, "/v1/media",
		map[string]any{"content_type": "application/pdf", "size_bytes": 10}, hdrs)
	require.Equal(t, http.StatusBadRequest, bad.Code)

	hdrs["Idempotency-Key"] = uuid.NewString()
	created := postJSON(t, h, "/v1/media",
		map[string]any{"content_type": "image/png", "size_bytes": 10}, hdrs)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var ticket struct {
		MediaId   string `json:"media_id"`
		UploadUrl string `json:"upload_url"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &ticket))
	require.NotEmpty(t, ticket.UploadUrl)

	conflict := postJSON(t, h, "/v1/media/"+ticket.MediaId+"/complete", nil,
		map[string]string{"Authorization": "Bearer " + tok})
	require.Equal(t, http.StatusConflict, conflict.Code, "complete before upload")

	req := httptest.NewRequest(http.MethodGet, "/v1/media/"+ticket.MediaId, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "pending")
}
