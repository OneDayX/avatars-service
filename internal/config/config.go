package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPAddr          string
	MetricsAddr       string
	DatabaseURL       string
	MigrationsDir     string
	RunMigrations     bool
	WebDir            string
	S3Endpoint        string
	S3AccessKey       string
	S3SecretKey       string
	S3Bucket          string
	S3UseSSL          bool
	RabbitMQURL       string
	MaxFileSize       int64
	RateLimitRequests int
	RateLimitWindow   time.Duration
	AllowedOrigins    string
	ShutdownTimeout   time.Duration
	WorkerMaxRetries  int
	WorkerRetryDelay  time.Duration
	OTelEnabled       bool
	OTLPEndpoint      string
	ServiceName       string
	Environment       string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:          env("HTTP_ADDR", ":8080"),
		MetricsAddr:       env("METRICS_ADDR", ":8081"),
		DatabaseURL:       env("DATABASE_URL", "postgres://gophprofile:gophprofile@localhost:5432/gophprofile?sslmode=disable"),
		MigrationsDir:     env("MIGRATIONS_DIR", "migrations"),
		WebDir:            env("WEB_DIR", "web"),
		S3Endpoint:        env("S3_ENDPOINT", "localhost:9000"),
		S3AccessKey:       env("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:       env("S3_SECRET_KEY", "minioadmin"),
		S3Bucket:          env("S3_BUCKET", "avatars"),
		RabbitMQURL:       env("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		MaxFileSize:       10 << 20,
		RateLimitRequests: 60,
		RateLimitWindow:   time.Minute,
		AllowedOrigins:    env("CORS_ALLOWED_ORIGINS", "*"),
		ShutdownTimeout:   10 * time.Second,
		WorkerMaxRetries:  5,
		WorkerRetryDelay:  500 * time.Millisecond,
		OTLPEndpoint:      env("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		ServiceName:       env("OTEL_SERVICE_NAME", "gophprofile"),
		Environment:       env("APP_ENV", "development"),
	}
	var err error
	if cfg.S3UseSSL, err = boolEnv("S3_USE_SSL", false); err != nil {
		return Config{}, err
	}
	if cfg.OTelEnabled, err = boolEnv("OTEL_ENABLED", false); err != nil {
		return Config{}, err
	}
	if cfg.RunMigrations, err = boolEnv("RUN_MIGRATIONS", true); err != nil {
		return Config{}, err
	}
	if cfg.MaxFileSize, err = int64Env("MAX_FILE_SIZE", cfg.MaxFileSize); err != nil {
		return Config{}, err
	}
	if cfg.RateLimitRequests, err = intEnv("RATE_LIMIT_REQUESTS", cfg.RateLimitRequests); err != nil {
		return Config{}, err
	}
	if cfg.RateLimitWindow, err = durationEnv("RATE_LIMIT_WINDOW", cfg.RateLimitWindow); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = durationEnv("SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout); err != nil {
		return Config{}, err
	}
	if cfg.WorkerMaxRetries, err = intEnv("WORKER_MAX_RETRIES", cfg.WorkerMaxRetries); err != nil {
		return Config{}, err
	}
	if cfg.WorkerRetryDelay, err = durationEnv("WORKER_RETRY_DELAY", cfg.WorkerRetryDelay); err != nil {
		return Config{}, err
	}
	if cfg.MaxFileSize <= 0 || cfg.RateLimitRequests <= 0 || cfg.RateLimitWindow <= 0 || cfg.WorkerMaxRetries < 0 || cfg.WorkerRetryDelay < 0 {
		return Config{}, fmt.Errorf("limits and durations must be positive")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func boolEnv(key string, fallback bool) (bool, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func intEnv(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func int64Env(key string, fallback int64) (int64, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}
