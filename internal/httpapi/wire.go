package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/fonvacano/yaxter/internal/auth"
	"github.com/fonvacano/yaxter/pkg/idem"
	"github.com/fonvacano/yaxter/pkg/redisx"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

type Deps struct {
	DB            *pgxpool.Pool
	Redis         *redis.Client
	IDs           *snowflake.Generator
	JWTKid        string
	JWTSeed       []byte
	AuthRateLimit int
}

// idemSkip exempts token-issuance routes from Idempotency-Key (deviation #4
// in the plan header / OpenAPI contract).
func idemSkip(r *http.Request) bool {
	switch r.URL.Path {
	case "/v1/auth/login", "/v1/auth/refresh", "/v1/auth/logout":
		return true
	}
	return strings.HasPrefix(r.URL.Path, "/v1/auth/oauth/")
}

// NewHandler assembles the full /v1 handler with the middleware stack:
// request-id -> auth-route limit -> idempotency -> bearer auth -> routes.
func NewHandler(d Deps) (http.Handler, error) {
	issuer, err := auth.NewTokenIssuer(d.JWTKid, d.JWTSeed, 15*time.Minute)
	if err != nil {
		return nil, err
	}
	svc := auth.NewService(d.DB, d.IDs, issuer,
		auth.NewRefreshStore(d.DB, d.IDs, 30*24*time.Hour))
	srv := NewServer(svc)

	h := HandlerWithOptions(srv, StdHTTPServerOptions{BaseURL: "/v1"})
	h = BearerAuth(issuer.Verify)(h)
	h = idem.New(d.Redis, 24*time.Hour).Skip(idemSkip).Wrap(h)
	h = AuthRateLimit(redisx.NewLimiter(d.Redis), d.AuthRateLimit, time.Minute)(h)
	h = RequestID(h)
	return h, nil
}
