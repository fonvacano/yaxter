package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "yaxter", cfg.ServiceName)
	require.Equal(t, "dev", cfg.Env)
	require.Equal(t, "info", cfg.LogLevel)
	require.Equal(t, ":8080", cfg.HTTPAddr)
	require.Equal(t, "localhost:6379", cfg.RedisAddr)
	require.Empty(t, cfg.WorkerRoles)
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("KAFKA_BROKERS", "k1:9092,k2:9092")
	t.Setenv("WORKER_ROLES", "relay,fanout")
	t.Setenv("POSTGRES_DSN", "postgres://u:p@h:5432/db")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "debug", cfg.LogLevel)
	require.Equal(t, []string{"k1:9092", "k2:9092"}, cfg.KafkaBrokers)
	require.Equal(t, []string{"relay", "fanout"}, cfg.WorkerRoles)
	require.Equal(t, "postgres://u:p@h:5432/db", cfg.PostgresDSN)
}

func TestRelayConfigDefaults(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 200*time.Millisecond, cfg.RelayPollInterval)
	require.Equal(t, 500, cfg.RelayBatchSize)
}
