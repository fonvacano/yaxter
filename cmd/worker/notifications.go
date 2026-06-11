package main

import (
	"context"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/fonvacano/yaxter/internal/notifications"
	"github.com/fonvacano/yaxter/pkg/config"
	"github.com/fonvacano/yaxter/pkg/kafkax"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/redisx"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

func init() { roleRunners["notifications"] = runNotifications }

func hostnameOr(fallback string) string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return fallback
}

func runNotifications(ctx context.Context, logger zerolog.Logger, cfg config.Config) {
	pool, err := pgxkit.NewPool(ctx, cfg.PostgresDSN)
	if err != nil {
		logger.Error().Err(err).Msg("notifications: pg pool")
		return
	}
	defer pool.Close()
	lease, err := snowflake.AcquireLease(ctx, pool, hostnameOr("worker-notifications"), 30*time.Second)
	if err != nil {
		logger.Error().Err(err).Msg("notifications: snowflake lease")
		return
	}
	gen, err := snowflake.New(lease.NodeID())
	if err != nil {
		logger.Error().Err(err).Msg("notifications: snowflake gen")
		return
	}
	rdb := redisx.NewClient(cfg.RedisAddr)
	client, err := kafkax.NewClient(cfg.KafkaBrokers,
		kgo.ConsumerGroup(kafkax.GroupID("notifications")),
		kgo.ConsumeTopics("follows.v1", "engagements.v1"))
	if err != nil {
		logger.Error().Err(err).Msg("notifications: kafka client")
		return
	}
	defer client.Close()
	w := notifications.NewWorker(pool, rdb, gen)
	logger.Info().Msg("notifications worker started")
	if err := kafkax.Consume(ctx, client, logger, w.HandleRecord); err != nil {
		logger.Error().Err(err).Msg("notifications: consume loop ended")
	}
}
