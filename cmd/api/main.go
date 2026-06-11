// Command api is the stateless HTTP service: REST endpoints, auth, timeline
// reads, write acks (ARCHITECTURE.md §1.1). Routes are added by phase-1/2
// tasks; T0.1 ships config/log/otel wiring, /healthz, graceful shutdown.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"github.com/fonvacano/yaxter/internal/auth/oauth"
	"github.com/fonvacano/yaxter/internal/httpapi"
	"github.com/fonvacano/yaxter/internal/media"
	"github.com/fonvacano/yaxter/pkg/config"
	logkit "github.com/fonvacano/yaxter/pkg/log"
	otelkit "github.com/fonvacano/yaxter/pkg/otel"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/redisx"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	logger := logkit.New(os.Stdout, cfg.LogLevel, "api")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownOtel, err := otelkit.Setup(ctx, otelkit.Config{
		ServiceName: "yaxter-api",
		Endpoint:    cfg.OTLPEndpoint,
		SampleRatio: cfg.SampleRatio,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("otel setup failed")
	}

	pool, err := pgxkit.NewPool(ctx, cfg.PostgresDSN)
	if err != nil {
		logger.Fatal().Err(err).Msg("postgres unreachable")
	}
	defer pool.Close()

	lease, err := snowflake.AcquireLease(ctx, pool, hostnameOr("api"), 30*time.Second)
	if err != nil {
		logger.Fatal().Err(err).Msg("snowflake lease")
	}
	go heartbeatLoop(ctx, logger, lease, 10*time.Second)
	gen, err := snowflake.New(lease.NodeID())
	if err != nil {
		logger.Fatal().Err(err).Msg("snowflake generator")
	}

	seed, err := jwtSeed(cfg, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("jwt seed")
	}
	mediaStore, err := media.NewStore(ctx, media.StoreConfig{
		Endpoint:     cfg.S3Endpoint,
		Region:       cfg.S3Region,
		AccessKey:    cfg.S3AccessKeyID,
		SecretKey:    cfg.S3SecretAccessKey,
		Bucket:       cfg.S3MediaBucket,
		UsePathStyle: cfg.S3UsePathStyle,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("media store init")
	}
	if err := mediaStore.EnsureBucket(ctx); err != nil {
		logger.Warn().Err(err).Msg("media bucket ensure failed - presign may still work")
	}

	oauthProviders := buildOAuthProviders(ctx, cfg, logger)

	apiHandler, err := httpapi.NewHandler(httpapi.Deps{
		DB:                 pool,
		Redis:              redisx.NewClient(cfg.RedisAddr),
		IDs:                gen,
		JWTKid:             cfg.AuthJWTKid,
		JWTSeed:            seed,
		AuthRateLimit:      cfg.AuthRateLimit,
		CelebrityThreshold: cfg.CelebrityThreshold,
		MediaBaseURL:       cfg.MediaBaseURL,
		MediaStore:         mediaStore,
		OAuthProviders:     oauthProviders,
		OAuthRedirectBase:  cfg.OAuthRedirectBase,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("handler wiring")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/v1/", apiHandler)

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info().Str("addr", cfg.HTTPAddr).Msg("api listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal().Err(err).Msg("http server failed")
		}
	}()

	<-ctx.Done()
	logger.Info().Msg("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	_ = shutdownOtel(shutCtx)
}

// buildOAuthProviders constructs the enabled OAuth provider adapters from
// config. Providers listed without credentials (or whose discovery fails) are
// logged and skipped, so the service still boots (config drives enablement).
func buildOAuthProviders(ctx context.Context, cfg config.Config, logger zerolog.Logger) map[string]oauth.Provider {
	out := map[string]oauth.Provider{}
	for _, name := range cfg.OAuthProviders {
		switch name {
		case "yandex":
			if cfg.YandexClientID == "" {
				logger.Warn().Msg("yandex oauth listed but no credentials - disabled")
				continue
			}
			out["yandex"] = oauth.NewYandex(cfg.YandexClientID, cfg.YandexClientSecret,
				oauth.YandexEndpoints{AuthURL: cfg.YandexAuthURL, TokenURL: cfg.YandexTokenURL, InfoURL: cfg.YandexInfoURL})
		case "google":
			if cfg.GoogleClientID == "" {
				logger.Warn().Msg("google oauth listed but no credentials - disabled")
				continue
			}
			g, err := oauth.NewGoogle(ctx, cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleIssuer)
			if err != nil {
				logger.Error().Err(err).Msg("google oidc discovery failed - disabled")
				continue
			}
			out["google"] = g
		}
	}
	return out
}

func hostnameOr(fallback string) string {
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return fallback
}

func heartbeatLoop(ctx context.Context, logger zerolog.Logger, lease *snowflake.Lease, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := lease.Heartbeat(ctx); err != nil {
				logger.Fatal().Err(err).Msg("snowflake lease lost")
			}
		}
	}
}

// jwtSeed decodes the configured seed, or generates an ephemeral one with a
// loud warning — tokens won't survive restarts (dev convenience only).
func jwtSeed(cfg config.Config, logger zerolog.Logger) ([]byte, error) {
	if cfg.AuthJWTSeedB64 != "" {
		return base64.StdEncoding.DecodeString(cfg.AuthJWTSeedB64)
	}
	logger.Warn().Msg("AUTH_JWT_SEED_B64 not set - using ephemeral jwt key (dev only)")
	seed := make([]byte, ed25519.SeedSize)
	_, err := rand.Read(seed)
	return seed, err
}
