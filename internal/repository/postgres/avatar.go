package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/example/gophprofile/internal/domain"
	"github.com/example/gophprofile/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Pool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
	Ping(ctx context.Context) error
}

type AvatarRepository struct {
	pool Pool
}

func NewAvatarRepository(pool Pool) *AvatarRepository {
	return &AvatarRepository{pool: pool}
}

const avatarColumns = `id, user_id, file_name, mime_type, size_bytes, s3_key,
	thumbnail_s3_keys, upload_status, processing_status, width, height,
	created_at, updated_at, deleted_at`

func (r *AvatarRepository) Create(ctx context.Context, avatar *domain.Avatar) error {
	query := `INSERT INTO avatars (
		id, user_id, file_name, mime_type, size_bytes, s3_key, thumbnail_s3_keys,
		upload_status, processing_status, width, height, created_at, updated_at
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`
	_, err := r.pool.Exec(ctx, query,
		avatar.ID, avatar.UserID, avatar.FileName, avatar.MIMEType, avatar.SizeBytes,
		avatar.S3Key, avatar.ThumbnailS3Keys, avatar.UploadStatus, avatar.ProcessingStatus,
		avatar.Dimensions.Width, avatar.Dimensions.Height, avatar.CreatedAt, avatar.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert avatar: %w", err)
	}
	return nil
}

func (r *AvatarRepository) DeleteByID(ctx context.Context, id uuid.UUID) error {
	result, err := r.pool.Exec(ctx, `DELETE FROM avatars WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete avatar: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *AvatarRepository) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*domain.Avatar, error) {
	query := `SELECT ` + avatarColumns + ` FROM avatars WHERE id = $1`
	if !includeDeleted {
		query += ` AND deleted_at IS NULL`
	}
	return r.get(ctx, query, id)
}

func (r *AvatarRepository) GetLatestByUser(ctx context.Context, userID string) (*domain.Avatar, error) {
	query := `SELECT ` + avatarColumns + ` FROM avatars
		WHERE user_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1`
	return r.get(ctx, query, userID)
}

func (r *AvatarRepository) get(ctx context.Context, query string, args ...any) (*domain.Avatar, error) {
	avatar, err := scanAvatar(r.pool.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get avatar: %w", err)
	}
	return avatar, nil
}

func (r *AvatarRepository) ListByUser(ctx context.Context, userID string) ([]domain.Avatar, error) {
	query := `SELECT ` + avatarColumns + ` FROM avatars
		WHERE user_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list avatars: %w", err)
	}
	defer rows.Close()
	avatars := make([]domain.Avatar, 0)
	for rows.Next() {
		avatar, err := scanAvatar(rows)
		if err != nil {
			return nil, fmt.Errorf("scan avatar: %w", err)
		}
		avatars = append(avatars, *avatar)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate avatars: %w", err)
	}
	return avatars, nil
}

func (r *AvatarRepository) StorageUsage(ctx context.Context) ([]domain.UserStorageUsage, error) {
	rows, err := r.pool.Query(ctx, `SELECT user_id, SUM(size_bytes) FROM avatars WHERE deleted_at IS NULL GROUP BY user_id`)
	if err != nil {
		return nil, fmt.Errorf("calculate storage usage: %w", err)
	}
	defer rows.Close()
	usage := make([]domain.UserStorageUsage, 0)
	for rows.Next() {
		var item domain.UserStorageUsage
		if err := rows.Scan(&item.UserID, &item.Bytes); err != nil {
			return nil, fmt.Errorf("scan storage usage: %w", err)
		}
		usage = append(usage, item)
	}
	return usage, rows.Err()
}

func (r *AvatarRepository) UpdateProcessingStatus(ctx context.Context, id uuid.UUID, status string, thumbnails map[string]string) error {
	if thumbnails == nil {
		thumbnails = map[string]string{}
	}
	query := `UPDATE avatars SET processing_status = $2, thumbnail_s3_keys = $3, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`
	result, err := r.pool.Exec(ctx, query, id, status, thumbnails)
	if err != nil {
		return fmt.Errorf("update processing status: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *AvatarRepository) SoftDelete(ctx context.Context, id uuid.UUID, userID string) (*domain.Avatar, error) {
	query := `UPDATE avatars SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL RETURNING ` + avatarColumns
	avatar, err := scanAvatar(r.pool.QueryRow(ctx, query, id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soft delete avatar: %w", err)
	}
	return avatar, nil
}

func (r *AvatarRepository) SoftDeleteLatest(ctx context.Context, userID string) (*domain.Avatar, error) {
	query := `UPDATE avatars SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = (SELECT id FROM avatars WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT 1) RETURNING ` + avatarColumns
	avatar, err := scanAvatar(r.pool.QueryRow(ctx, query, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soft delete latest avatar: %w", err)
	}
	return avatar, nil
}

func (r *AvatarRepository) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAvatar(row scanner) (*domain.Avatar, error) {
	var avatar domain.Avatar
	err := row.Scan(
		&avatar.ID, &avatar.UserID, &avatar.FileName, &avatar.MIMEType, &avatar.SizeBytes,
		&avatar.S3Key, &avatar.ThumbnailS3Keys, &avatar.UploadStatus, &avatar.ProcessingStatus,
		&avatar.Dimensions.Width, &avatar.Dimensions.Height,
		&avatar.CreatedAt, &avatar.UpdatedAt, &avatar.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	if avatar.ThumbnailS3Keys == nil {
		avatar.ThumbnailS3Keys = map[string]string{}
	}
	return &avatar, nil
}

var _ repository.AvatarRepository = (*AvatarRepository)(nil)
