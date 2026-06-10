// Command seed populates the demo dataset. T0.5 ships the harness skeleton:
// it verifies the dev stack is reachable and migrated. The full dataset
// (1k users, Zipf follow graph, ~5 celebrities, 20k tweets) lands in T4.2.
package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/fonvacano/yaxter/pkg/config"
	logkit "github.com/fonvacano/yaxter/pkg/log"
	pgxkit "github.com/fonvacano/yaxter/pkg/pgx"
	"github.com/fonvacano/yaxter/pkg/redisx"
)

func main() {
	users := flag.Int("users", 1000, "users to create (dataset arrives with T4.2)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	logger := logkit.New(os.Stdout, cfg.LogLevel, "seed")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dsn := cfg.PostgresDSN
	if dsn == "" {
		dsn = "postgres://yaxter:yaxter@localhost:5432/yaxter?sslmode=disable"
	}
	pool, err := pgxkit.NewPool(ctx, dsn)
	if err != nil {
		logger.Fatal().Err(err).Msg("postgres unreachable - run `make up` first")
	}
	defer pool.Close()

	var version int
	var dirty bool
	if err := pool.QueryRow(ctx,
		`SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty); err != nil || dirty {
		logger.Fatal().Err(err).Bool("dirty", dirty).Msg("migrations not applied cleanly")
	}

	rdb := redisx.NewClient(cfg.RedisAddr)
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Fatal().Err(err).Msg("redis unreachable")
	}

	logger.Info().
		Int("schema_version", version).
		Int("users_requested", *users).
		Msg("seed harness ready - full dataset generation arrives with T4.2")
}
