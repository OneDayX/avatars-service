package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/example/gophprofile/internal/broker/rabbitmq"
	"github.com/example/gophprofile/internal/config"
	"github.com/example/gophprofile/internal/observability"
	postgresrepo "github.com/example/gophprofile/internal/repository/postgres"
	basestorage "github.com/example/gophprofile/internal/storage"
	miniostorage "github.com/example/gophprofile/internal/storage/minio"
	"github.com/example/gophprofile/internal/worker"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	logger := observability.NewLogger("gophprofile-worker")
	if err := run(logger); err != nil {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownTracing, err := observability.SetupTracing(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		_ = shutdownTracing(shutdownCtx)
	}()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	db := postgresrepo.NewTracedPool(pool)
	repo := postgresrepo.NewAvatarRepository(db)
	s3, err := miniostorage.New(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3UseSSL)
	if err != nil {
		return err
	}
	storage := basestorage.NewTracedStorage(s3)
	broker, err := rabbitmq.New(cfg.RabbitMQURL)
	if err != nil {
		return err
	}
	defer func() { _ = broker.Close() }()
	prometheus.MustRegister(
		observability.NewStorageUsageCollector(repo),
		observability.NewInfrastructureCollector(pool, broker),
	)
	metricsServer := &http.Server{Addr: cfg.MetricsAddr, Handler: promhttp.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.ErrorContext(ctx, "metrics server stopped", "error", err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		_ = metricsServer.Shutdown(shutdownCtx)
	}()
	avatarWorker := worker.New(repo, storage, broker, cfg.WorkerMaxRetries, cfg.WorkerRetryDelay, logger.With("component", "worker"))
	logger.Info("worker started", "metrics_address", cfg.MetricsAddr)
	return avatarWorker.Run(ctx)
}
