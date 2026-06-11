package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

func liveHandler(t *testing.T, rateLimit int) http.Handler {
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

	mr := miniredis.RunT(t)
	gen, err := snowflake.New(3)
	require.NoError(t, err)
	h, err := NewHandler(Deps{
		DB:    pool,
		Redis: redis.NewClient(&redis.Options{Addr: mr.Addr()}),
		IDs:   gen, JWTKid: "test-1",
		JWTSeed:       bytes.Repeat([]byte{7}, 32),
		AuthRateLimit: rateLimit,
	})
	require.NoError(t, err)
	return h
}

// liveHandlerAndPool is like liveHandler but also returns the pgxpool for
// direct assertion queries within the same test.
func liveHandlerAndPool(t *testing.T, rateLimit int) (http.Handler, *pgxpool.Pool) {
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

	mr := miniredis.RunT(t)
	gen, err := snowflake.New(4)
	require.NoError(t, err)
	h, err := NewHandler(Deps{
		DB:    pool,
		Redis: redis.NewClient(&redis.Options{Addr: mr.Addr()}),
		IDs:   gen, JWTKid: "test-1",
		JWTSeed:       bytes.Repeat([]byte{7}, 32),
		AuthRateLimit: rateLimit,
	})
	require.NoError(t, err)
	return h, pool
}

func postJSON(t *testing.T, h http.Handler, path string, body map[string]any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.1.1.1:1000"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestFullTokenLifecycleOverHTTP(t *testing.T) {
	h := liveHandler(t, 100)

	// Register (requires Idempotency-Key per contract).
	reg := postJSON(t, h, "/v1/auth/register", map[string]any{
		"username": "carol", "email": "carol@example.com", "password": "password123",
	}, map[string]string{"Idempotency-Key": "8f7c0e84-2a51-4b9d-9d6f-1234567890ab"})
	require.Equal(t, http.StatusCreated, reg.Code, reg.Body.String())

	var regBody struct {
		Tokens struct {
			AccessToken  string  `json:"access_token"`
			RefreshToken *string `json:"refresh_token"`
		} `json:"tokens"`
	}
	require.NoError(t, json.Unmarshal(reg.Body.Bytes(), &regBody))
	require.NotEmpty(t, regBody.Tokens.AccessToken)
	require.NotNil(t, regBody.Tokens.RefreshToken)

	// Login without Idempotency-Key must work (skip predicate).
	login := postJSON(t, h, "/v1/auth/login",
		map[string]any{"login": "carol", "password": "password123"}, nil)
	require.Equal(t, http.StatusOK, login.Code, login.Body.String())

	// Refresh rotates.
	refresh1 := postJSON(t, h, "/v1/auth/refresh",
		map[string]any{"refresh_token": *regBody.Tokens.RefreshToken}, nil)
	require.Equal(t, http.StatusOK, refresh1.Code)
	var pair1 struct {
		RefreshToken *string `json:"refresh_token"`
	}
	require.NoError(t, json.Unmarshal(refresh1.Body.Bytes(), &pair1))

	// Reuse of the pre-rotation token → 401, and the family dies.
	reuse := postJSON(t, h, "/v1/auth/refresh",
		map[string]any{"refresh_token": *regBody.Tokens.RefreshToken}, nil)
	require.Equal(t, http.StatusUnauthorized, reuse.Code)
	dead := postJSON(t, h, "/v1/auth/refresh",
		map[string]any{"refresh_token": *pair1.RefreshToken}, nil)
	require.Equal(t, http.StatusUnauthorized, dead.Code)
}

func TestAuthRateLimitOverHTTP(t *testing.T) {
	h := liveHandler(t, 3)
	var last *httptest.ResponseRecorder
	for i := 0; i < 4; i++ {
		last = postJSON(t, h, "/v1/auth/login",
			map[string]any{"login": fmt.Sprintf("u%d", i), "password": "x"}, nil)
	}
	require.Equal(t, http.StatusTooManyRequests, last.Code)
	require.NotEmpty(t, last.Header().Get("Retry-After"))
}

func TestUnimplementedRoutesReturn501(t *testing.T) {
	h := liveHandler(t, 100)
	// /v1/media is T1.5 — still unimplemented
	req := httptest.NewRequest(http.MethodPost, "/v1/media", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotImplemented, rr.Code)
}

func getJSON(t *testing.T, h http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func registerAndLogin(t *testing.T, h http.Handler, username string) string {
	t.Helper()
	postJSON(t, h, "/v1/auth/register", map[string]any{
		"username": username,
		"email":    username + "@test.io",
		"password": "password123",
	}, nil)
	rr := postJSON(t, h, "/v1/auth/login", map[string]any{
		"login":    username + "@test.io",
		"password": "password123",
	}, nil)
	var resp struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	return resp.Tokens.AccessToken
}
