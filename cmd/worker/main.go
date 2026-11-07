package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/gophprofile/internal/broker/rabbitmq"
	"github.com/example/gophprofile/internal/config"
	postgresrepo "github.com/example/gophprofile/internal/repository/postgres"
	miniostorage "github.com/example/gophprofile/internal/storage/minio"
	"github.com/example/gophprofile/internal/worker"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
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
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	repo := postgresrepo.NewAvatarRepository(pool)
	storage, err := miniostorage.New(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3UseSSL)
	if err != nil {
		return err
	}
	broker, err := rabbitmq.New(cfg.RabbitMQURL)
	if err != nil {
		return err
	}
	defer func() { _ = broker.Close() }()
	avatarWorker := worker.New(repo, storage, broker, cfg.WorkerMaxRetries, cfg.WorkerRetryDelay, logger.With("component", "worker"))
	logger.Info("worker started")
	return avatarWorker.Run(ctx)
}
