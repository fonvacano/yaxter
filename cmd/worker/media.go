package main

import (
	"context"

	"github.com/rs/zerolog"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/fonvacano/yaxter/internal/media"
	"github.com/fonvacano/yaxter/pkg/config"
	"github.com/fonvacano/yaxter/pkg/kafkax"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/snowflake"
)

func init() { roleRunners["media"] = runMedia }

func runMedia(ctx context.Context, logger zerolog.Logger, cfg config.Config) {
	log := logger.With().Str("role", "media").Logger()
	pool, err := pgxkit.NewPool(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatal().Err(err).Msg("postgres unreachable")
	}
	defer pool.Close()

	store, err := media.NewStore(ctx, media.StoreConfig{
		Endpoint: cfg.S3Endpoint, Region: cfg.S3Region,
		AccessKey: cfg.S3AccessKeyID, SecretKey: cfg.S3SecretAccessKey,
		Bucket: cfg.S3MediaBucket, UsePathStyle: cfg.S3UsePathStyle,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("s3 store")
	}
	gen, err := snowflake.New(0) // worker generates no ids on this path; lease not needed yet
	if err != nil {
		log.Fatal().Err(err).Msg("snowflake")
	}
	svc := media.NewService(pool, store, gen)

	client, err := kafkax.NewClient(cfg.KafkaBrokers,
		kgo.ConsumerGroup(kafkax.GroupID("media")),
		kgo.ConsumeTopics("media.v1"))
	if err != nil {
		log.Fatal().Err(err).Msg("kafka client")
	}
	defer client.Close()

	if err := kafkax.Consume(ctx, client, log, svc.HandleRecord); err != nil && ctx.Err() == nil {
		log.Fatal().Err(err).Msg("media consumer exited")
	}
}
