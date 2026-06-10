package httpapi

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	logkit "github.com/fonvacano/yaxter/pkg/log"
	"github.com/fonvacano/yaxter/pkg/redisx"
)

type userIDKey struct{}

// UserID returns the authenticated user id from the context.
func UserID(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey{}).(int64)
	return id, ok
}

// RequestID honors an incoming X-Request-Id (set by the ALB) or generates
// one, stores it in the context for logging, and echoes it on the response.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(logkit.WithRequestID(r.Context(), id)))
	})
}

// BearerAuth verifies a present Bearer token and stores the user id in the
// context. Absent header passes through anonymously — per-route auth
// enforcement happens in handlers (ownership checks, §7). An invalid token
// is always a 401: it signals a broken or expired client session.
func BearerAuth(verify func(string) (int64, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				next.ServeHTTP(w, r)
				return
			}
			token, ok := strings.CutPrefix(header, "Bearer ")
			if !ok {
				writeError(w, http.StatusUnauthorized, "invalid_token", "malformed Authorization header")
				return
			}
			uid, err := verify(token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid_token", "invalid or expired token")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey{}, uid)))
		})
	}
}

// AuthRateLimit applies a per-IP sliding window to /v1/auth/* routes
// (brute-force protection, §7). Other routes pass through.
func AuthRateLimit(l *redisx.Limiter, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/v1/auth/") {
				next.ServeHTTP(w, r)
				return
			}
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
			allowed, err := l.Allow(r.Context(), ip+":auth", limit, window)
			if err != nil { // Redis down: fail open, §2.8 keeps auth stateless
				next.ServeHTTP(w, r)
				return
			}
			if !allowed {
				w.Header().Set("Retry-After", strconv.Itoa(int(window.Seconds())))
				writeError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
