package idem

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newHandler(t *testing.T) (http.Handler, *atomic.Int32, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	var calls atomic.Int32
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":123}`))
	})
	return New(rdb, 24*time.Hour).Wrap(inner), &calls, mr
}

func do(t *testing.T, h http.Handler, method, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/v1/tweets", nil)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestMissingKeyRejected(t *testing.T) {
	h, calls, _ := newHandler(t)
	rr := do(t, h, http.MethodPost, "")
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Zero(t, calls.Load())
}

func TestInvalidKeyRejected(t *testing.T) {
	h, calls, _ := newHandler(t)
	rr := do(t, h, http.MethodPost, "not-a-uuid")
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Zero(t, calls.Load())
}

func TestGetBypasses(t *testing.T) {
	h, calls, _ := newHandler(t)
	rr := do(t, h, http.MethodGet, "")
	require.Equal(t, http.StatusCreated, rr.Code)
	require.EqualValues(t, 1, calls.Load())
}

func TestDuplicateReplaysCachedResponse(t *testing.T) {
	h, calls, _ := newHandler(t)
	key := uuid.NewString()

	first := do(t, h, http.MethodPost, key)
	require.Equal(t, http.StatusCreated, first.Code)

	second := do(t, h, http.MethodPost, key)
	require.Equal(t, http.StatusCreated, second.Code)
	b, _ := io.ReadAll(second.Body)
	require.JSONEq(t, `{"id":123}`, string(b))
	require.Equal(t, "application/json", second.Header().Get("Content-Type"))
	require.EqualValues(t, 1, calls.Load(), "handler must run exactly once")
}

func TestSkipPredicateBypassesMutatingRoute(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	var calls atomic.Int32
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	mw := New(rdb, 24*time.Hour).Skip(func(r *http.Request) bool {
		return r.URL.Path == "/v1/auth/login"
	})
	h := mw.Wrap(inner)

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil) // no Idempotency-Key
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.EqualValues(t, 1, calls.Load())

	// Non-skipped mutating routes still require the key.
	req = httptest.NewRequest(http.MethodPost, "/v1/tweets", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestInFlightDuplicateConflicts(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	release := make(chan struct{})
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusCreated)
	})
	h := New(rdb, 24*time.Hour).Wrap(inner)
	key := uuid.NewString()

	done := make(chan *httptest.ResponseRecorder)
	go func() {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/tweets", nil)
		req.Header.Set("Idempotency-Key", key)
		h.ServeHTTP(rr, req)
		done <- rr
	}()

	// Wait until the first request holds the in-flight lock.
	require.Eventually(t, func() bool {
		return mr.Exists("idem:" + key + ":lock")
	}, time.Second, 5*time.Millisecond)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/tweets", nil)
	req.Header.Set("Idempotency-Key", key)
	h.ServeHTTP(rr, req)
	require.Equal(t, http.StatusConflict, rr.Code)

	close(release)
	require.Equal(t, http.StatusCreated, (<-done).Code)
}
