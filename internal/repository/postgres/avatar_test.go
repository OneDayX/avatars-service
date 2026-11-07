package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/example/gophprofile/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
)

var columns = []string{"id", "user_id", "file_name", "mime_type", "size_bytes", "s3_key", "thumbnail_s3_keys", "upload_status", "processing_status", "width", "height", "created_at", "updated_at", "deleted_at"}

func avatarValues(avatar domain.Avatar) []any {
	return []any{avatar.ID, avatar.UserID, avatar.FileName, avatar.MIMEType, avatar.SizeBytes, avatar.S3Key, map[string]string{"100x100": "thumb"}, avatar.UploadStatus, avatar.ProcessingStatus, avatar.Dimensions.Width, avatar.Dimensions.Height, avatar.CreatedAt, avatar.UpdatedAt, avatar.DeletedAt}
}

func sampleAvatar() domain.Avatar {
	return domain.Avatar{ID: uuid.New(), UserID: "user", FileName: "me.png", MIMEType: "image/png", SizeBytes: 20, S3Key: "key", UploadStatus: "uploaded", ProcessingStatus: "pending", Dimensions: domain.Dimensions{Width: 10, Height: 20}, CreatedAt: time.Now(), UpdatedAt: time.Now()}
}

func TestCreate(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	repo := NewAvatarRepository(mock)
	avatar := sampleAvatar()
	mock.ExpectExec("INSERT INTO avatars").WithArgs(avatar.ID, avatar.UserID, avatar.FileName, avatar.MIMEType, avatar.SizeBytes, avatar.S3Key, avatar.ThumbnailS3Keys, avatar.UploadStatus, avatar.ProcessingStatus, avatar.Dimensions.Width, avatar.Dimensions.Height, avatar.CreatedAt, avatar.UpdatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	require.NoError(t, repo.Create(context.Background(), &avatar))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteByID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	repo := NewAvatarRepository(mock)
	id := uuid.New()
	mock.ExpectExec("DELETE FROM avatars WHERE id").WithArgs(id).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	require.NoError(t, repo.DeleteByID(context.Background(), id))
	mock.ExpectExec("DELETE FROM avatars WHERE id").WithArgs(id).WillReturnResult(pgxmock.NewResult("DELETE", 0))
	require.ErrorIs(t, repo.DeleteByID(context.Background(), id), domain.ErrNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetByID(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	repo := NewAvatarRepository(mock)
	avatar := sampleAvatar()
	mock.ExpectQuery("SELECT .* FROM avatars WHERE id = \\$1 AND deleted_at IS NULL").WithArgs(avatar.ID).WillReturnRows(pgxmock.NewRows(columns).AddRow(avatarValues(avatar)...))
	result, err := repo.GetByID(context.Background(), avatar.ID, false)
	require.NoError(t, err)
	require.Equal(t, avatar.ID, result.ID)
	require.Equal(t, "thumb", result.ThumbnailS3Keys["100x100"])
	mock.ExpectQuery("SELECT .* FROM avatars").WithArgs(avatar.ID).WillReturnError(pgx.ErrNoRows)
	_, err = repo.GetByID(context.Background(), avatar.ID, true)
	require.ErrorIs(t, err, domain.ErrNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetLatestAndList(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	repo := NewAvatarRepository(mock)
	avatar := sampleAvatar()
	mock.ExpectQuery("SELECT .* ORDER BY created_at DESC LIMIT 1").WithArgs(avatar.UserID).WillReturnRows(pgxmock.NewRows(columns).AddRow(avatarValues(avatar)...))
	latest, err := repo.GetLatestByUser(context.Background(), avatar.UserID)
	require.NoError(t, err)
	require.Equal(t, avatar.ID, latest.ID)
	mock.ExpectQuery("SELECT .* ORDER BY created_at DESC").WithArgs(avatar.UserID).WillReturnRows(pgxmock.NewRows(columns).AddRow(avatarValues(avatar)...))
	list, err := repo.ListByUser(context.Background(), avatar.UserID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStorageUsage(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	repo := NewAvatarRepository(mock)
	mock.ExpectQuery("SELECT user_id, SUM").WillReturnRows(pgxmock.NewRows([]string{"user_id", "bytes"}).AddRow("user", int64(1024)))
	usage, err := repo.StorageUsage(context.Background())
	require.NoError(t, err)
	require.Equal(t, []domain.UserStorageUsage{{UserID: "user", Bytes: 1024}}, usage)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateProcessingStatus(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	repo := NewAvatarRepository(mock)
	id := uuid.New()
	thumbnails := map[string]string{"100x100": "thumb"}
	mock.ExpectExec("UPDATE avatars SET processing_status").WithArgs(id, domain.ProcessingCompleted, thumbnails).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	require.NoError(t, repo.UpdateProcessingStatus(context.Background(), id, domain.ProcessingCompleted, thumbnails))
	mock.ExpectExec("UPDATE avatars SET processing_status").WithArgs(id, domain.ProcessingFailed, map[string]string{}).WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	require.ErrorIs(t, repo.UpdateProcessingStatus(context.Background(), id, domain.ProcessingFailed, nil), domain.ErrNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSoftDelete(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	repo := NewAvatarRepository(mock)
	avatar := sampleAvatar()
	now := time.Now()
	avatar.DeletedAt = &now
	mock.ExpectQuery("UPDATE avatars SET deleted_at").WithArgs(avatar.ID, avatar.UserID).WillReturnRows(pgxmock.NewRows(columns).AddRow(avatarValues(avatar)...))
	deleted, err := repo.SoftDelete(context.Background(), avatar.ID, avatar.UserID)
	require.NoError(t, err)
	require.NotNil(t, deleted.DeletedAt)
	mock.ExpectQuery("UPDATE avatars SET deleted_at").WithArgs(avatar.ID, avatar.UserID).WillReturnError(pgx.ErrNoRows)
	_, err = repo.SoftDelete(context.Background(), avatar.ID, avatar.UserID)
	require.ErrorIs(t, err, domain.ErrNotFound)
	mock.ExpectQuery("UPDATE avatars SET deleted_at").WithArgs(avatar.UserID).WillReturnRows(pgxmock.NewRows(columns).AddRow(avatarValues(avatar)...))
	_, err = repo.SoftDeleteLatest(context.Background(), avatar.UserID)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepositoryErrorsAndPing(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	repo := NewAvatarRepository(mock)
	avatar := sampleAvatar()
	dbErr := errors.New("database down")
	mock.ExpectExec("INSERT INTO avatars").WithArgs(
		pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
		pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
		pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
	).WillReturnError(dbErr)
	require.ErrorIs(t, repo.Create(context.Background(), &avatar), dbErr)
	mock.ExpectQuery("SELECT .* FROM avatars").WithArgs("user").WillReturnError(dbErr)
	_, err = repo.ListByUser(context.Background(), "user")
	require.ErrorIs(t, err, dbErr)
	mock.ExpectPing().WillReturnError(dbErr)
	require.ErrorIs(t, repo.Ping(context.Background()), dbErr)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMigrate(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	directory := t.TempDir()
	name := "000001_test.up.sql"
	require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte("CREATE TABLE test_table (id INT);"), 0o600))
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_lock(727272)")).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(pgxmock.NewResult("CREATE", 0))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(name).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE test_table").WillReturnResult(pgxmock.NewResult("CREATE", 0))
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(name).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectExec(regexp.QuoteMeta("SELECT pg_advisory_unlock(727272)")).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	require.NoError(t, Migrate(context.Background(), mock, directory))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMigrateSkipsApplied(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()
	directory := t.TempDir()
	name := "000001_test.up.sql"
	require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte("SELECT 1;"), 0o600))
	mock.ExpectExec("SELECT pg_advisory_lock").WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(pgxmock.NewResult("CREATE", 0))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(name).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("SELECT pg_advisory_unlock").WillReturnResult(pgxmock.NewResult("SELECT", 1))
	require.NoError(t, Migrate(context.Background(), mock, directory))
	require.NoError(t, mock.ExpectationsWereMet())
}
