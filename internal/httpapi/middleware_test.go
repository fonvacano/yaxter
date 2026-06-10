package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	logkit "github.com/fonvacano/yaxter/pkg/log"
	"github.com/fonvacano/yaxter/pkg/redisx"
)

func TestRequestIDMiddleware(t *testing.T) {
	var captured string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = logkit.RequestID(r.Context())
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	require.NotEmpty(t, captured)
	require.Equal(t, captured, rr.Header().Get("X-Request-Id"))

	// Incoming header is honored (ALB generates upstream ids).
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", "upstream-1")
	h.ServeHTTP(httptest.NewRecorder(), req)
	require.Equal(t, "upstream-1", captured)
}

func TestBearerAuthMiddleware(t *testing.T) {
	verify := func(token string) (int64, error) {
		if token == "good" {
			return 42, nil
		}
		return 0, http.ErrNoCookie // any error
	}
	var uid int64
	var ok bool
	h := BearerAuth(verify)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, ok = UserID(r.Context())
	}))

	// No header: passes through unauthenticated (handlers decide per-route).
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	require.False(t, ok)

	// Valid token: user id lands in context.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer good")
	h.ServeHTTP(httptest.NewRecorder(), req)
	require.True(t, ok)
	require.EqualValues(t, 42, uid)

	// Invalid token: 401 immediately (don't run the handler as anonymous).
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer bad")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestAuthRouteIPLimit(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	limiter := redisx.NewLimiter(rdb)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := AuthRateLimit(limiter, 2, time.Minute)(inner)

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusTooManyRequests, rr.Code)
	require.NotEmpty(t, rr.Header().Get("Retry-After"))

	// Other IPs unaffected; non-auth paths unaffected.
	req = httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	req.RemoteAddr = "10.0.0.2:1234"
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	req = httptest.NewRequest(http.MethodPost, "/v1/tweets", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
}
