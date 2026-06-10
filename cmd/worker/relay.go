package main

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/fonvacano/yaxter/internal/relay"
	"github.com/fonvacano/yaxter/pkg/config"
	"github.com/fonvacano/yaxter/pkg/kafkax"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
)

func init() { roleRunners["relay"] = runRelay }

func runRelay(ctx context.Context, logger zerolog.Logger, cfg config.Config) {
	pool, err := pgxkit.NewPool(ctx, cfg.PostgresDSN)
	if err != nil {
		logger.Fatal().Err(err).Msg("relay: postgres unreachable")
	}
	defer pool.Close()

	client, err := kafkax.NewClient(cfg.KafkaBrokers)
	if err != nil {
		logger.Fatal().Err(err).Msg("relay: kafka client")
	}
	defer client.Close()

	rcfg := relay.DefaultConfig()
	rcfg.PollInterval = cfg.RelayPollInterval
	rcfg.BatchSize = cfg.RelayBatchSize

	r := relay.New(pool, relay.NewKafkaPublisher(client), rcfg,
		relay.NewMetrics(metricsRegistry), logger.With().Str("role", "relay").Logger())
	if err := r.Run(ctx); err != nil && ctx.Err() == nil {
		logger.Fatal().Err(err).Msg("relay exited")
	}
}
