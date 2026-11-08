package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/gophprofile/internal/config"
	"github.com/example/gophprofile/internal/observability"
	postgresrepo "github.com/example/gophprofile/internal/repository/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := observability.NewLogger("gophprofile-migrate")
	if err := run(); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
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
	return postgresrepo.Migrate(ctx, postgresrepo.NewTracedPool(pool), cfg.MigrationsDir)
}
