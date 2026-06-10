// Command worker runs the role-selected background workers: outbox relay and
// Kafka consumers (ARCHITECTURE.md §1.1). Roles are placeholders until their
// phase-1/2 tasks land.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	"github.com/fonvacano/yaxter/pkg/config"
	logkit "github.com/fonvacano/yaxter/pkg/log"
	otelkit "github.com/fonvacano/yaxter/pkg/otel"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	logger := logkit.New(os.Stdout, cfg.LogLevel, "worker")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownOtel, err := otelkit.Setup(ctx, otelkit.Config{
		ServiceName: "yaxter-worker",
		Endpoint:    cfg.OTLPEndpoint,
		SampleRatio: cfg.SampleRatio,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("otel setup failed")
	}

	roles, err := resolveRoles(cfg.WorkerRoles)
	if err != nil {
		logger.Fatal().Err(err).Msg("invalid WORKER_ROLES")
	}
	logger.Info().Strs("roles", roles).Msg("worker starting")

	for _, role := range roles {
		if runner, ok := roleRunners[role]; ok {
			go runner(ctx, logger, cfg)
			continue
		}
		go runRole(ctx, logger, role)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("GET /metrics", promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{}))
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
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

// metricsRegistry is shared by all roles; /metrics is served from it.
var metricsRegistry = prometheus.NewRegistry()

// runRole is a placeholder loop; each role is replaced by its real
// implementation in T1.0 (relay), T2.1 (fanout), T1.4 (counters),
// T2.3 (notifications), T1.5 (media).
func runRole(ctx context.Context, logger zerolog.Logger, role string) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	logger.Info().Str("role", role).Msg("role runner started (placeholder)")
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			logger.Debug().Str("role", role).Msg("role heartbeat")
		}
	}
}
