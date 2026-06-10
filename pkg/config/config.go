// Package config loads service configuration from environment variables.
// Demo and production differ only in env values, never in code (ARCHITECTURE.md core principle).
package config

import "github.com/caarlos0/env/v11"

type Config struct {
	ServiceName  string   `env:"SERVICE_NAME" envDefault:"yaxter"`
	Env          string   `env:"APP_ENV" envDefault:"dev"`
	LogLevel     string   `env:"LOG_LEVEL" envDefault:"info"`
	HTTPAddr     string   `env:"HTTP_ADDR" envDefault:":8080"`
	PostgresDSN  string   `env:"POSTGRES_DSN"`
	RedisAddr    string   `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	KafkaBrokers []string `env:"KAFKA_BROKERS" envSeparator:","`
	WorkerRoles  []string `env:"WORKER_ROLES" envSeparator:","`
	OTLPEndpoint   string  `env:"OTLP_ENDPOINT"`
	SampleRatio    float64 `env:"OTEL_SAMPLE_RATIO" envDefault:"1.0"`
	AuthJWTKid     string  `env:"AUTH_JWT_KID" envDefault:"dev-1"`
	AuthJWTSeedB64 string  `env:"AUTH_JWT_SEED_B64"` // 32-byte ed25519 seed, base64; empty => ephemeral (dev only)
	AuthRateLimit  int     `env:"AUTH_RATE_LIMIT" envDefault:"20"` // per IP per window
}

func Load() (Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return Config{}, err
	}
	return c, nil
}
