package main

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/fonvacano/yaxter/internal/counters"
	"github.com/fonvacano/yaxter/pkg/config"
	"github.com/fonvacano/yaxter/pkg/kafkax"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/redisx"
)

func init() { roleRunners["counters"] = runCounters }

func runCounters(ctx context.Context, logger zerolog.Logger, cfg config.Config) {
	log := logger.With().Str("role", "counters").Logger()
	pool, err := pgxkit.NewPool(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatal().Err(err).Msg("postgres unreachable")
	}
	defer pool.Close()
	rdb := redisx.NewClient(cfg.RedisAddr)
	defer rdb.Close()

	client, err := kafkax.NewClient(cfg.KafkaBrokers,
		kgo.ConsumerGroup(kafkax.GroupID("counters")),
		kgo.ConsumeTopics("engagements.v1"))
	if err != nil {
		log.Fatal().Err(err).Msg("kafka client")
	}
	defer client.Close()

	c := counters.New(pool, rdb, 500, 2*time.Second)
	go c.Run(ctx)
	go func() {
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := counters.Reconcile(ctx, pool, rdb); err != nil {
					log.Error().Err(err).Msg("reconcile failed")
				}
			}
		}
	}()

	err = kafkax.Consume(ctx, client, log, c.HandleRecord)
	if err != nil && ctx.Err() == nil {
		log.Fatal().Err(err).Msg("counters consumer exited")
	}
}
