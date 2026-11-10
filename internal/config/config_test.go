package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{"HTTP_ADDR", "DATABASE_URL", "S3_USE_SSL", "MAX_FILE_SIZE", "RATE_LIMIT_REQUESTS", "RATE_LIMIT_WINDOW", "WORKER_MAX_RETRIES", "WORKER_RETRY_DELAY"} {
		t.Setenv(key, "")
	}
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, int64(10<<20), cfg.MaxFileSize)
	require.Equal(t, "avatars", cfg.S3Bucket)
	require.True(t, cfg.RunMigrations)
	require.Equal(t, time.Minute, cfg.RateLimitWindow)
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("MAX_FILE_SIZE", "1024")
	t.Setenv("S3_USE_SSL", "true")
	t.Setenv("RATE_LIMIT_REQUESTS", "5")
	t.Setenv("RATE_LIMIT_WINDOW", "2s")
	t.Setenv("WORKER_MAX_RETRIES", "2")
	t.Setenv("WORKER_RETRY_DELAY", "10ms")
	t.Setenv("SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("RUN_MIGRATIONS", "false")
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, int64(1024), cfg.MaxFileSize)
	require.True(t, cfg.S3UseSSL)
	require.Equal(t, 5, cfg.RateLimitRequests)
	require.Equal(t, 2*time.Second, cfg.RateLimitWindow)
	require.Equal(t, 3*time.Second, cfg.ShutdownTimeout)
	require.False(t, cfg.RunMigrations)
}

func TestLoadMigrationIgnoresApplicationConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://migration-db/avatars")
	t.Setenv("MIGRATIONS_DIR", "db/migrations")
	t.Setenv("MAX_FILE_SIZE", "invalid")
	t.Setenv("S3_USE_SSL", "invalid")
	t.Setenv("RATE_LIMIT_WINDOW", "invalid")
	cfg := LoadMigration()
	require.Equal(t, "postgres://migration-db/avatars", cfg.DatabaseURL)
	require.Equal(t, "db/migrations", cfg.MigrationsDir)
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	for _, test := range []struct{ key, value string }{{"S3_USE_SSL", "maybe"}, {"MAX_FILE_SIZE", "large"}, {"RATE_LIMIT_WINDOW", "soon"}, {"MAX_FILE_SIZE", "0"}} {
		t.Run(test.key+test.value, func(t *testing.T) {
			t.Setenv(test.key, test.value)
			_, err := Load()
			require.Error(t, err)
		})
	}
}
