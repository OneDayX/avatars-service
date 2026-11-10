package main

import (
	"context"
	"fmt"
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
	cfg := config.LoadMigration()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("create PostgreSQL connection pool: %w", err)
	}
	defer pool.Close()
	if err := postgresrepo.Migrate(ctx, postgresrepo.NewTracedPool(pool), cfg.MigrationsDir); err != nil {
		return fmt.Errorf("apply migrations from %s: %w", cfg.MigrationsDir, err)
	}
	return nil
}
