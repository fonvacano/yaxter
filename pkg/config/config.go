// Package config loads service configuration from environment variables.
// Demo and production differ only in env values, never in code (ARCHITECTURE.md core principle).
package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	ServiceName        string        `env:"SERVICE_NAME" envDefault:"yaxter"`
	Env                string        `env:"APP_ENV" envDefault:"dev"`
	LogLevel           string        `env:"LOG_LEVEL" envDefault:"info"`
	HTTPAddr           string        `env:"HTTP_ADDR" envDefault:":8080"`
	PostgresDSN        string        `env:"POSTGRES_DSN"`
	RedisAddr          string        `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	KafkaBrokers       []string      `env:"KAFKA_BROKERS" envSeparator:","`
	WorkerRoles        []string      `env:"WORKER_ROLES" envSeparator:","`
	OTLPEndpoint       string        `env:"OTLP_ENDPOINT"`
	SampleRatio        float64       `env:"OTEL_SAMPLE_RATIO" envDefault:"1.0"`
	RelayPollInterval  time.Duration `env:"RELAY_POLL_INTERVAL" envDefault:"200ms"`
	RelayBatchSize     int           `env:"RELAY_BATCH_SIZE" envDefault:"500"`
	AuthJWTKid         string        `env:"AUTH_JWT_KID" envDefault:"dev-1"`
	AuthJWTSeedB64     string        `env:"AUTH_JWT_SEED_B64"`
	AuthRateLimit      int           `env:"AUTH_RATE_LIMIT" envDefault:"20"`
	CelebrityThreshold int           `env:"CELEBRITY_THRESHOLD" envDefault:"50"`
	MediaBaseURL       string        `env:"MEDIA_BASE_URL" envDefault:"http://localhost:9000/media"`
	S3Endpoint         string        `env:"S3_ENDPOINT" envDefault:"http://localhost:9000"`
	S3Region           string        `env:"S3_REGION" envDefault:"ru-central1"`
	S3AccessKeyID      string        `env:"S3_ACCESS_KEY_ID" envDefault:"yaxter"`
	S3SecretAccessKey  string        `env:"S3_SECRET_ACCESS_KEY" envDefault:"yaxter123"`
	S3MediaBucket      string        `env:"S3_MEDIA_BUCKET" envDefault:"media"`
	S3UsePathStyle     bool          `env:"S3_USE_PATH_STYLE" envDefault:"true"`

	OAuthProviders     []string `env:"OAUTH_PROVIDERS" envSeparator:"," envDefault:"yandex"`
	OAuthRedirectBase  string   `env:"OAUTH_REDIRECT_BASE" envDefault:"http://localhost:8080"`
	YandexClientID     string   `env:"OAUTH_YANDEX_CLIENT_ID"`
	YandexClientSecret string   `env:"OAUTH_YANDEX_CLIENT_SECRET"`
	YandexAuthURL      string   `env:"OAUTH_YANDEX_AUTH_URL"`  // test/dev override
	YandexTokenURL     string   `env:"OAUTH_YANDEX_TOKEN_URL"` // test/dev override
	YandexInfoURL      string   `env:"OAUTH_YANDEX_INFO_URL"`  // test/dev override
	GoogleClientID     string   `env:"OAUTH_GOOGLE_CLIENT_ID"`
	GoogleClientSecret string   `env:"OAUTH_GOOGLE_CLIENT_SECRET"`
	GoogleIssuer       string   `env:"OAUTH_GOOGLE_ISSUER"` // mock override; default accounts.google.com
}

func Load() (Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return Config{}, err
	}
	return c, nil
}
