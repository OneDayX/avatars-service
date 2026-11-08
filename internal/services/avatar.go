package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/example/gophprofile/internal/broker"
	"github.com/example/gophprofile/internal/domain"
	"github.com/example/gophprofile/internal/images"
	"github.com/example/gophprofile/internal/metrics"
	"github.com/example/gophprofile/internal/repository"
	"github.com/example/gophprofile/internal/storage"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type AvatarService struct {
	repo        repository.AvatarRepository
	storage     storage.Storage
	publisher   broker.Publisher
	maxFileSize int64
}

func NewAvatarService(repo repository.AvatarRepository, storage storage.Storage, publisher broker.Publisher, maxFileSize int64) *AvatarService {
	return &AvatarService{repo: repo, storage: storage, publisher: publisher, maxFileSize: maxFileSize}
}

func validateUserID(userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" || len(userID) > 255 || !utf8.ValidString(userID) || strings.ContainsAny(userID, "\x00\r\n\t") {
		return "", domain.ErrInvalidUserID
	}
	return userID, nil
}

func (s *AvatarService) Upload(ctx context.Context, userID, fileName string, data []byte) (result *domain.Avatar, err error) {
	started := time.Now()
	status := "success"
	ctx, span := otel.Tracer("gophprofile/avatars").Start(ctx, "avatar.upload")
	defer func() {
		if err != nil {
			status = "error"
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		metrics.UploadsTotal.WithLabelValues(status).Inc()
		metrics.UploadDuration.WithLabelValues(status).Observe(time.Since(started).Seconds())
		span.End()
	}()
	span.SetAttributes(attribute.String("user_id", userID), attribute.String("file_name", fileName), attribute.Int("file_size", len(data)))
	userID, err = validateUserID(userID)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > s.maxFileSize {
		return nil, domain.ErrFileTooLarge
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty file", domain.ErrInvalidFile)
	}
	decoded, err := images.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidFile, err)
	}
	fileName = filepath.Base(strings.TrimSpace(fileName))
	if fileName == "" || fileName == "." || fileName == string(filepath.Separator) {
		fileName = "avatar"
	}
	if len(fileName) > 255 {
		fileName = fileName[len(fileName)-255:]
	}
	now := time.Now().UTC()
	avatar := &domain.Avatar{
		ID: uuid.New(), UserID: userID, FileName: fileName, MIMEType: decoded.MIMEType,
		SizeBytes: int64(len(data)), Dimensions: domain.Dimensions{Width: decoded.Width, Height: decoded.Height},
		ThumbnailS3Keys: map[string]string{}, UploadStatus: "uploaded", ProcessingStatus: domain.ProcessingPending,
		CreatedAt: now, UpdatedAt: now,
	}
	extension := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}[decoded.MIMEType]
	avatar.S3Key = "avatars/" + avatar.ID.String() + "/original" + extension
	if err := s.storage.Upload(ctx, avatar.S3Key, avatar.MIMEType, data); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, avatar); err != nil {
		_ = s.storage.Delete(context.Background(), avatar.S3Key)
		return nil, err
	}
	event := domain.AvatarUploadEvent{AvatarID: avatar.ID.String(), UserID: avatar.UserID, S3Key: avatar.S3Key}
	if err := s.publisher.PublishUpload(ctx, event); err != nil {
		publishErr := fmt.Errorf("publish upload event: %w", err)
		if rollbackErr := s.rollbackUpload(context.WithoutCancel(ctx), avatar); rollbackErr != nil {
			return nil, errors.Join(publishErr, fmt.Errorf("rollback upload: %w", rollbackErr))
		}
		return nil, publishErr
	}
	return avatar, nil
}

func (s *AvatarService) rollbackUpload(ctx context.Context, avatar *domain.Avatar) error {
	var rollbackErrors []error
	if err := s.repo.DeleteByID(ctx, avatar.ID); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("delete avatar metadata: %w", err))
	}
	if err := s.storage.Delete(ctx, avatar.S3Key); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("delete original file: %w", err))
	}
	return errors.Join(rollbackErrors...)
}

func (s *AvatarService) Get(ctx context.Context, id uuid.UUID, size string) (*domain.Avatar, storage.Object, error) {
	avatar, err := s.repo.GetByID(ctx, id, false)
	if err != nil {
		return nil, storage.Object{}, err
	}
	return s.download(ctx, avatar, size)
}

func (s *AvatarService) GetLatest(ctx context.Context, userID, size string) (*domain.Avatar, storage.Object, error) {
	userID, err := validateUserID(userID)
	if err != nil {
		return nil, storage.Object{}, err
	}
	avatar, err := s.repo.GetLatestByUser(ctx, userID)
	if err != nil {
		return nil, storage.Object{}, err
	}
	return s.download(ctx, avatar, size)
}

func (s *AvatarService) download(ctx context.Context, avatar *domain.Avatar, size string) (*domain.Avatar, storage.Object, error) {
	if size == "" {
		size = "original"
	}
	key := avatar.S3Key
	contentType := avatar.MIMEType
	if size != "original" {
		if size != "100x100" && size != "300x300" {
			return nil, storage.Object{}, domain.ErrInvalidSize
		}
		thumbnailKey, ok := avatar.ThumbnailS3Keys[size]
		if !ok {
			return nil, storage.Object{}, domain.ErrNotReady
		}
		key = thumbnailKey
		contentType = "image/jpeg"
	}
	object, err := s.storage.Download(ctx, key)
	if err != nil {
		return nil, storage.Object{}, err
	}
	if object.ContentType == "" {
		object.ContentType = contentType
	}
	return avatar, object, nil
}

func (s *AvatarService) Metadata(ctx context.Context, id uuid.UUID) (*domain.Avatar, error) {
	return s.repo.GetByID(ctx, id, false)
}

func (s *AvatarService) List(ctx context.Context, userID string) ([]domain.Avatar, error) {
	userID, err := validateUserID(userID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListByUser(ctx, userID)
}

func (s *AvatarService) Delete(ctx context.Context, id uuid.UUID, userID string) error {
	userID, err := validateUserID(userID)
	if err != nil {
		return err
	}
	avatar, err := s.repo.GetByID(ctx, id, false)
	if err != nil {
		return err
	}
	if avatar.UserID != userID {
		return domain.ErrForbidden
	}
	deleted, err := s.repo.SoftDelete(ctx, id, userID)
	if err != nil {
		return err
	}
	return s.publishDelete(ctx, deleted)
}

func (s *AvatarService) DeleteLatest(ctx context.Context, userID string) error {
	userID, err := validateUserID(userID)
	if err != nil {
		return err
	}
	deleted, err := s.repo.SoftDeleteLatest(ctx, userID)
	if err != nil {
		return err
	}
	return s.publishDelete(ctx, deleted)
}

func (s *AvatarService) publishDelete(ctx context.Context, avatar *domain.Avatar) error {
	keys := make([]string, 0, len(avatar.ThumbnailS3Keys)+1)
	keys = append(keys, avatar.S3Key)
	for _, key := range avatar.ThumbnailS3Keys {
		keys = append(keys, key)
	}
	event := domain.AvatarDeleteEvent{AvatarID: avatar.ID.String(), S3Keys: keys}
	if err := s.publisher.PublishDelete(ctx, event); err != nil {
		return fmt.Errorf("publish delete event: %w", err)
	}
	return nil
}
