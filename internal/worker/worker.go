package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/example/gophprofile/internal/broker"
	"github.com/example/gophprofile/internal/domain"
	"github.com/example/gophprofile/internal/images"
	"github.com/example/gophprofile/internal/repository"
	"github.com/example/gophprofile/internal/storage"
	"github.com/google/uuid"
)

type Worker struct {
	repo       repository.AvatarRepository
	storage    storage.Storage
	consumer   broker.Consumer
	maxRetries int
	retryDelay time.Duration
	logger     *slog.Logger
	sleep      func(context.Context, time.Duration) error
}

func New(repo repository.AvatarRepository, storage storage.Storage, consumer broker.Consumer, maxRetries int, retryDelay time.Duration, logger *slog.Logger) *Worker {
	if logger == nil {
		logger = slog.Default()
	}
	return &Worker{repo: repo, storage: storage, consumer: consumer, maxRetries: maxRetries, retryDelay: retryDelay, logger: logger, sleep: sleepContext}
}

func (w *Worker) Run(ctx context.Context) error {
	deliveries, err := w.consumer.Consume(ctx)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("broker delivery channel closed")
			}
			err := w.processWithRetry(ctx, delivery.Event)
			if err == nil {
				if err := delivery.Ack(); err != nil {
					return err
				}
				continue
			}
			w.logger.ErrorContext(ctx, "event processing failed", "type", delivery.Event.Type, "error", err)
			if errors.Is(ctx.Err(), context.Canceled) {
				_ = delivery.Reject(true)
				return nil
			}
			if err := delivery.Reject(false); err != nil {
				return err
			}
		}
	}
}

func (w *Worker) processWithRetry(ctx context.Context, event broker.Event) error {
	var err error
	for attempt := 0; attempt <= w.maxRetries; attempt++ {
		if err = w.Process(ctx, event); err == nil {
			return nil
		}
		if attempt == w.maxRetries || ctx.Err() != nil {
			break
		}
		w.logger.WarnContext(ctx, "retrying event", "type", event.Type, "attempt", attempt+1, "error", err)
		if sleepErr := w.sleep(ctx, w.retryDelay<<attempt); sleepErr != nil {
			return sleepErr
		}
	}
	if event.Type == broker.EventUpload {
		if id, parseErr := uuid.Parse(event.Upload.AvatarID); parseErr == nil {
			avatar, getErr := w.repo.GetByID(context.Background(), id, true)
			if getErr == nil && avatar.DeletedAt == nil {
				_ = w.repo.UpdateProcessingStatus(context.Background(), id, domain.ProcessingFailed, avatar.ThumbnailS3Keys)
			}
		}
	}
	return err
}

func (w *Worker) Process(ctx context.Context, event broker.Event) error {
	switch event.Type {
	case broker.EventUpload:
		return w.processUpload(ctx, event.Upload)
	case broker.EventDelete:
		return w.processDelete(ctx, event.Delete)
	default:
		return fmt.Errorf("unsupported event type %q", event.Type)
	}
}

func (w *Worker) processUpload(ctx context.Context, event domain.AvatarUploadEvent) error {
	id, err := uuid.Parse(event.AvatarID)
	if err != nil {
		return fmt.Errorf("invalid avatar id: %w", err)
	}
	avatar, err := w.repo.GetByID(ctx, id, true)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return err
	}
	if avatar.DeletedAt != nil {
		return nil
	}
	if avatar.ProcessingStatus == domain.ProcessingCompleted && avatar.ThumbnailS3Keys["100x100"] != "" && avatar.ThumbnailS3Keys["300x300"] != "" {
		return nil
	}
	if err := w.repo.UpdateProcessingStatus(ctx, id, domain.ProcessingProcessing, avatar.ThumbnailS3Keys); err != nil {
		return err
	}
	object, err := w.storage.Download(ctx, event.S3Key)
	if err != nil {
		return err
	}
	data, readErr := io.ReadAll(object.Body)
	closeErr := object.Body.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	decoded, err := images.Decode(data)
	if err != nil {
		return err
	}
	thumbnails := make(map[string]string, 2)
	for _, size := range []struct {
		name          string
		width, height int
	}{{"100x100", 100, 100}, {"300x300", 300, 300}} {
		data, err := images.Thumbnail(decoded.Image, size.width, size.height)
		if err != nil {
			return err
		}
		key := fmt.Sprintf("thumbnails/%s/%s.jpg", event.AvatarID, size.name)
		if err := w.storage.Upload(ctx, key, "image/jpeg", data); err != nil {
			return err
		}
		thumbnails[size.name] = key
	}
	return w.repo.UpdateProcessingStatus(ctx, id, domain.ProcessingCompleted, thumbnails)
}

func (w *Worker) processDelete(ctx context.Context, event domain.AvatarDeleteEvent) error {
	seen := make(map[string]struct{}, len(event.S3Keys))
	for _, key := range event.S3Keys {
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if err := w.storage.Delete(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
