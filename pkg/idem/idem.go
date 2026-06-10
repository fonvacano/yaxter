// Package idem enforces Idempotency-Key on mutating endpoints
// (ARCHITECTURE.md §7): 24h Redis dedupe window, replay of the original
// response, 409 for concurrent duplicates. The durable Postgres tier and
// per-user key scoping arrive with T1.3/T1.1; the key layout already
// reserves them.
package idem

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const lockTTL = 30 * time.Second

type record struct {
	Status int         `json:"status"`
	Header http.Header `json:"header"`
	Body   []byte      `json:"body"`
}

type Middleware struct {
	rdb *redis.Client
	ttl time.Duration
}

func New(rdb *redis.Client, ttl time.Duration) *Middleware {
	return &Middleware{rdb: rdb, ttl: ttl}
}

// recorder tees the response to the client while buffering it for storage.
type recorder struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (r *recorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	r.buf.Write(p)
	return r.ResponseWriter.Write(p)
}

func mutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

func (m *Middleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !mutating(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		ctx := r.Context()

		key := r.Header.Get("Idempotency-Key")
		if _, err := uuid.Parse(key); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_idempotency_key",
				"Idempotency-Key header must be a UUID")
			return
		}
		redisKey := "idem:" + key

		// Replay a completed duplicate.
		if data, err := m.rdb.Get(ctx, redisKey).Bytes(); err == nil {
			var rec record
			if json.Unmarshal(data, &rec) == nil {
				for k, vs := range rec.Header {
					for _, v := range vs {
						w.Header().Add(k, v)
					}
				}
				w.WriteHeader(rec.Status)
				_, _ = w.Write(rec.Body)
				return
			}
		}

		// Claim in-flight; a concurrent duplicate gets 409.
		ok, err := m.rdb.SetNX(ctx, redisKey+":lock", "1", lockTTL).Result()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "idempotency_unavailable",
				"idempotency store unavailable")
			return
		}
		if !ok {
			writeJSONError(w, http.StatusConflict, "request_in_flight",
				"a request with this Idempotency-Key is already being processed")
			return
		}

		rec := &recorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		stored, _ := json.Marshal(record{Status: rec.status, Header: rec.Header().Clone(), Body: rec.buf.Bytes()})
		m.rdb.Set(ctx, redisKey, stored, m.ttl)
		m.rdb.Del(ctx, redisKey+":lock")
	})
}

func writeJSONError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": msg})
}
