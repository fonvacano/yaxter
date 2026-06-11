package main

import (
	"context"

	"github.com/rs/zerolog"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/fonvacano/yaxter/internal/fanout"
	"github.com/fonvacano/yaxter/pkg/config"
	"github.com/fonvacano/yaxter/pkg/kafkax"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/redisx"
)

func init() { roleRunners["fanout"] = runFanout }

func runFanout(ctx context.Context, logger zerolog.Logger, cfg config.Config) {
	pool, err := pgxkit.NewPool(ctx, cfg.PostgresDSN)
	if err != nil {
		logger.Error().Err(err).Msg("fanout: pg pool")
		return
	}
	defer pool.Close()
	rdb := redisx.NewClient(cfg.RedisAddr)
	client, err := kafkax.NewClient(cfg.KafkaBrokers,
		kgo.ConsumerGroup(kafkax.GroupID("fanout")),
		kgo.ConsumeTopics("tweets.v1"))
	if err != nil {
		logger.Error().Err(err).Msg("fanout: kafka client")
		return
	}
	defer client.Close()
	f := fanout.New(pool, rdb, cfg.CelebrityThreshold, fanout.NewMetrics(metricsRegistry))
	logger.Info().Msg("fanout worker started")
	if err := kafkax.Consume(ctx, client, logger, f.HandleRecord); err != nil {
		logger.Error().Err(err).Msg("fanout: consume loop ended")
	}
}
