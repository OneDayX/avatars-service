package repository

import (
	"context"

	"github.com/example/gophprofile/internal/domain"
	"github.com/google/uuid"
)

type AvatarRepository interface {
	Create(ctx context.Context, avatar *domain.Avatar) error
	DeleteByID(ctx context.Context, id uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*domain.Avatar, error)
	GetLatestByUser(ctx context.Context, userID string) (*domain.Avatar, error)
	ListByUser(ctx context.Context, userID string) ([]domain.Avatar, error)
	UpdateProcessingStatus(ctx context.Context, id uuid.UUID, status string, thumbnails map[string]string) error
	SoftDelete(ctx context.Context, id uuid.UUID, userID string) (*domain.Avatar, error)
	SoftDeleteLatest(ctx context.Context, userID string) (*domain.Avatar, error)
	Ping(ctx context.Context) error
}
