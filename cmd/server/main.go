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
	"github.com/example/gophprofile/internal/handlers"
	postgresrepo "github.com/example/gophprofile/internal/repository/postgres"
	"github.com/example/gophprofile/internal/services"
	miniostorage "github.com/example/gophprofile/internal/storage/minio"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "error", err)
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
	if err := postgresrepo.Migrate(ctx, pool, cfg.MigrationsDir); err != nil {
		return err
	}
	repo := postgresrepo.NewAvatarRepository(pool)
	storage, err := miniostorage.New(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3UseSSL)
	if err != nil {
		return err
	}
	if err := storage.EnsureBucket(ctx); err != nil {
		return err
	}
	broker, err := rabbitmq.New(cfg.RabbitMQURL)
	if err != nil {
		return err
	}
	defer func() { _ = broker.Close() }()
	service := services.NewAvatarService(repo, storage, broker, cfg.MaxFileSize)
	avatarHandler := handlers.NewAvatarHandler(service, cfg.MaxFileSize)
	healthHandler := handlers.NewHealthHandler(repo, storage, broker)
	webHandler, err := handlers.NewWebHandler(service, cfg.WebDir, cfg.MaxFileSize)
	if err != nil {
		return err
	}
	limiter := handlers.NewRateLimiter(cfg.RateLimitRequests, cfg.RateLimitWindow)
	router := chi.NewRouter()
	router.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, middleware.Timeout(30*time.Second), handlers.CORS(cfg.AllowedOrigins))
	router.Get("/health", healthHandler.Check)
	router.Get("/", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/web/upload", http.StatusSeeOther) })
	router.Route("/api/v1", func(r chi.Router) {
		r.Use(limiter.Middleware)
		r.Post("/avatars", avatarHandler.Upload)
		r.Get("/avatars/{avatar_id}", avatarHandler.Get)
		r.Delete("/avatars/{avatar_id}", avatarHandler.Delete)
		r.Get("/avatars/{avatar_id}/metadata", avatarHandler.Metadata)
		r.Get("/users/{user_id}/avatar", avatarHandler.GetLatest)
		r.Delete("/users/{user_id}/avatar", avatarHandler.DeleteLatest)
		r.Get("/users/{user_id}/avatars", avatarHandler.List)
	})
	router.Get("/web/upload", webHandler.UploadForm)
	router.With(limiter.Middleware).Post("/web/upload", webHandler.Upload)
	router.Get("/web/gallery/{user_id}", webHandler.Gallery)
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: router, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	serveErrors := make(chan error, 1)
	go func() {
		logger.Info("HTTP server started", "address", cfg.HTTPAddr)
		serveErrors <- server.ListenAndServe()
	}()
	select {
	case err := <-serveErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return server.Shutdown(shutdownCtx)
}
