package services

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"testing"
	"time"

	"github.com/example/gophprofile/internal/broker"
	"github.com/example/gophprofile/internal/domain"
	"github.com/example/gophprofile/internal/storage"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fakeRepo struct {
	avatars   map[uuid.UUID]*domain.Avatar
	createErr error
	deleteErr error
}

func newFakeRepo() *fakeRepo { return &fakeRepo{avatars: map[uuid.UUID]*domain.Avatar{}} }
func (r *fakeRepo) Create(_ context.Context, a *domain.Avatar) error {
	if r.createErr != nil {
		return r.createErr
	}
	copy := *a
	r.avatars[a.ID] = &copy
	return nil
}
func (r *fakeRepo) DeleteByID(_ context.Context, id uuid.UUID) error {
	if _, ok := r.avatars[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.avatars, id)
	return nil
}
func (r *fakeRepo) GetByID(_ context.Context, id uuid.UUID, includeDeleted bool) (*domain.Avatar, error) {
	a, ok := r.avatars[id]
	if !ok || (a.DeletedAt != nil && !includeDeleted) {
		return nil, domain.ErrNotFound
	}
	copy := *a
	return &copy, nil
}
func (r *fakeRepo) GetLatestByUser(_ context.Context, userID string) (*domain.Avatar, error) {
	var latest *domain.Avatar
	for _, a := range r.avatars {
		if a.UserID == userID && a.DeletedAt == nil && (latest == nil || a.CreatedAt.After(latest.CreatedAt)) {
			latest = a
		}
	}
	if latest == nil {
		return nil, domain.ErrNotFound
	}
	copy := *latest
	return &copy, nil
}
func (r *fakeRepo) ListByUser(_ context.Context, userID string) ([]domain.Avatar, error) {
	result := []domain.Avatar{}
	for _, a := range r.avatars {
		if a.UserID == userID && a.DeletedAt == nil {
			result = append(result, *a)
		}
	}
	return result, nil
}
func (r *fakeRepo) UpdateProcessingStatus(_ context.Context, id uuid.UUID, status string, thumbnails map[string]string) error {
	a, ok := r.avatars[id]
	if !ok || a.DeletedAt != nil {
		return domain.ErrNotFound
	}
	a.ProcessingStatus = status
	a.ThumbnailS3Keys = thumbnails
	return nil
}
func (r *fakeRepo) SoftDelete(_ context.Context, id uuid.UUID, userID string) (*domain.Avatar, error) {
	if r.deleteErr != nil {
		return nil, r.deleteErr
	}
	a, err := r.GetByID(context.Background(), id, false)
	if err != nil || a.UserID != userID {
		return nil, domain.ErrNotFound
	}
	now := time.Now()
	a.DeletedAt = &now
	stored := r.avatars[id]
	stored.DeletedAt = &now
	return a, nil
}
func (r *fakeRepo) SoftDeleteLatest(_ context.Context, userID string) (*domain.Avatar, error) {
	a, err := r.GetLatestByUser(context.Background(), userID)
	if err != nil {
		return nil, err
	}
	return r.SoftDelete(context.Background(), a.ID, userID)
}
func (r *fakeRepo) Ping(context.Context) error { return nil }

type fakeStorage struct {
	objects   map[string][]byte
	uploadErr error
}

func newFakeStorage() *fakeStorage { return &fakeStorage{objects: map[string][]byte{}} }
func (s *fakeStorage) Upload(_ context.Context, key, _ string, data []byte) error {
	if s.uploadErr != nil {
		return s.uploadErr
	}
	s.objects[key] = append([]byte(nil), data...)
	return nil
}
func (s *fakeStorage) Download(_ context.Context, key string) (storage.Object, error) {
	data, ok := s.objects[key]
	if !ok {
		return storage.Object{}, errors.New("missing")
	}
	return storage.Object{Body: io.NopCloser(bytes.NewReader(data)), Size: int64(len(data))}, nil
}
func (s *fakeStorage) Delete(_ context.Context, key string) error { delete(s.objects, key); return nil }
func (s *fakeStorage) EnsureBucket(context.Context) error         { return nil }
func (s *fakeStorage) Ping(context.Context) error                 { return nil }

type fakePublisher struct {
	uploads []domain.AvatarUploadEvent
	deletes []domain.AvatarDeleteEvent
	err     error
}

func (p *fakePublisher) PublishUpload(_ context.Context, e domain.AvatarUploadEvent) error {
	if p.err != nil {
		return p.err
	}
	p.uploads = append(p.uploads, e)
	return nil
}
func (p *fakePublisher) PublishDelete(_ context.Context, e domain.AvatarDeleteEvent) error {
	if p.err != nil {
		return p.err
	}
	p.deletes = append(p.deletes, e)
	return nil
}
func (p *fakePublisher) Ping(context.Context) error { return nil }
func (p *fakePublisher) Close() error               { return nil }

func validPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 32, 16))))
	return buf.Bytes()
}
func setupService() (*AvatarService, *fakeRepo, *fakeStorage, *fakePublisher) {
	r := newFakeRepo()
	s := newFakeStorage()
	p := &fakePublisher{}
	return NewAvatarService(r, s, p, 1024*1024), r, s, p
}

func TestUpload(t *testing.T) {
	service, repo, store, publisher := setupService()
	avatar, err := service.Upload(context.Background(), " user@example.com ", "../me.png", validPNG(t))
	require.NoError(t, err)
	require.Equal(t, "user@example.com", avatar.UserID)
	require.Equal(t, "me.png", avatar.FileName)
	require.Equal(t, "image/png", avatar.MIMEType)
	require.Equal(t, 32, avatar.Dimensions.Width)
	require.Contains(t, store.objects, avatar.S3Key)
	require.Contains(t, repo.avatars, avatar.ID)
	require.Len(t, publisher.uploads, 1)
	require.Equal(t, avatar.S3Key, publisher.uploads[0].S3Key)
}

func TestUploadValidation(t *testing.T) {
	service, _, _, _ := setupService()
	_, err := service.Upload(context.Background(), "", "x", validPNG(t))
	require.ErrorIs(t, err, domain.ErrInvalidUserID)
	_, err = service.Upload(context.Background(), "user", "x", []byte("bad"))
	require.ErrorIs(t, err, domain.ErrInvalidFile)
	_, err = service.Upload(context.Background(), "user", "x", nil)
	require.ErrorIs(t, err, domain.ErrInvalidFile)
	service.maxFileSize = 2
	_, err = service.Upload(context.Background(), "user", "x", validPNG(t))
	require.ErrorIs(t, err, domain.ErrFileTooLarge)
}

func TestUploadRollbackAndPublishError(t *testing.T) {
	service, _, store, _ := setupService()
	service.repo.(*fakeRepo).createErr = errors.New("db down")
	_, err := service.Upload(context.Background(), "user", "x", validPNG(t))
	require.Error(t, err)
	require.Empty(t, store.objects)
	service, repo, store, publisher := setupService()
	publisher.err = errors.New("broker down")
	_, err = service.Upload(context.Background(), "user", "x", validPNG(t))
	require.ErrorContains(t, err, "publish upload event")
	require.Empty(t, repo.avatars)
	require.Empty(t, store.objects)
}

func TestGetVariants(t *testing.T) {
	service, repo, store, _ := setupService()
	avatar, err := service.Upload(context.Background(), "user", "x", validPNG(t))
	require.NoError(t, err)
	_, object, err := service.Get(context.Background(), avatar.ID, "original")
	require.NoError(t, err)
	require.NoError(t, object.Body.Close())
	_, _, err = service.Get(context.Background(), avatar.ID, "100x100")
	require.ErrorIs(t, err, domain.ErrNotReady)
	_, _, err = service.Get(context.Background(), avatar.ID, "20x20")
	require.ErrorIs(t, err, domain.ErrInvalidSize)
	stored := repo.avatars[avatar.ID]
	stored.ProcessingStatus = domain.ProcessingCompleted
	stored.ThumbnailS3Keys = map[string]string{"100x100": "thumb"}
	store.objects["thumb"] = []byte("jpg")
	_, object, err = service.Get(context.Background(), avatar.ID, "100x100")
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", object.ContentType)
	require.NoError(t, object.Body.Close())
	_, _, err = service.GetLatest(context.Background(), "user", "")
	require.NoError(t, err)
}

func TestMetadataAndList(t *testing.T) {
	service, _, _, _ := setupService()
	avatar, err := service.Upload(context.Background(), "user", "x", validPNG(t))
	require.NoError(t, err)
	metadata, err := service.Metadata(context.Background(), avatar.ID)
	require.NoError(t, err)
	require.Equal(t, avatar.ID, metadata.ID)
	list, err := service.List(context.Background(), "user")
	require.NoError(t, err)
	require.Len(t, list, 1)
	_, err = service.Metadata(context.Background(), uuid.New())
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDelete(t *testing.T) {
	service, repo, _, publisher := setupService()
	avatar, err := service.Upload(context.Background(), "user", "x", validPNG(t))
	require.NoError(t, err)
	repo.avatars[avatar.ID].ThumbnailS3Keys = map[string]string{"100x100": "thumb"}
	err = service.Delete(context.Background(), avatar.ID, "other")
	require.ErrorIs(t, err, domain.ErrForbidden)
	require.NoError(t, service.Delete(context.Background(), avatar.ID, "user"))
	require.Len(t, publisher.deletes, 1)
	require.ElementsMatch(t, []string{avatar.S3Key, "thumb"}, publisher.deletes[0].S3Keys)
	_, _, err = service.Get(context.Background(), avatar.ID, "")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestDeleteLatest(t *testing.T) {
	service, _, _, publisher := setupService()
	_, err := service.Upload(context.Background(), "user", "x", validPNG(t))
	require.NoError(t, err)
	require.NoError(t, service.DeleteLatest(context.Background(), "user"))
	require.Len(t, publisher.deletes, 1)
}

var _ broker.Publisher = (*fakePublisher)(nil)
