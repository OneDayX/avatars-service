package worker

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/example/gophprofile/internal/broker"
	"github.com/example/gophprofile/internal/domain"
	"github.com/example/gophprofile/internal/storage"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type repoStub struct {
	avatar    *domain.Avatar
	statuses  []string
	getErr    error
	updateErr error
}

func (r *repoStub) Create(context.Context, *domain.Avatar) error { return nil }
func (r *repoStub) DeleteByID(context.Context, uuid.UUID) error  { return nil }
func (r *repoStub) GetByID(context.Context, uuid.UUID, bool) (*domain.Avatar, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.avatar == nil {
		return nil, domain.ErrNotFound
	}
	copy := *r.avatar
	return &copy, nil
}
func (r *repoStub) GetLatestByUser(context.Context, string) (*domain.Avatar, error) { return nil, nil }
func (r *repoStub) ListByUser(context.Context, string) ([]domain.Avatar, error)     { return nil, nil }
func (r *repoStub) UpdateProcessingStatus(_ context.Context, _ uuid.UUID, status string, thumbs map[string]string) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.statuses = append(r.statuses, status)
	if r.avatar != nil {
		r.avatar.ProcessingStatus = status
		r.avatar.ThumbnailS3Keys = thumbs
	}
	return nil
}
func (r *repoStub) SoftDelete(context.Context, uuid.UUID, string) (*domain.Avatar, error) {
	return nil, nil
}
func (r *repoStub) SoftDeleteLatest(context.Context, string) (*domain.Avatar, error) { return nil, nil }
func (r *repoStub) Ping(context.Context) error                                       { return nil }

type storageStub struct {
	objects   map[string][]byte
	deleted   []string
	downloads int
	err       error
}

func (s *storageStub) Upload(_ context.Context, key, _ string, data []byte) error {
	if s.err != nil {
		return s.err
	}
	s.objects[key] = data
	return nil
}
func (s *storageStub) Download(_ context.Context, key string) (storage.Object, error) {
	s.downloads++
	if s.err != nil {
		return storage.Object{}, s.err
	}
	data, ok := s.objects[key]
	if !ok {
		return storage.Object{}, errors.New("missing")
	}
	return storage.Object{Body: io.NopCloser(bytes.NewReader(data)), Size: int64(len(data))}, nil
}
func (s *storageStub) Delete(_ context.Context, key string) error {
	if s.err != nil {
		return s.err
	}
	s.deleted = append(s.deleted, key)
	delete(s.objects, key)
	return nil
}
func (s *storageStub) EnsureBucket(context.Context) error { return nil }
func (s *storageStub) Ping(context.Context) error         { return nil }

func workerFixture() (*Worker, *repoStub, *storageStub, domain.AvatarUploadEvent) {
	id := uuid.New()
	avatar := &domain.Avatar{ID: id, UserID: "user", ProcessingStatus: domain.ProcessingPending, ThumbnailS3Keys: map[string]string{}}
	repo := &repoStub{avatar: avatar}
	store := &storageStub{objects: map[string][]byte{"original": pngDataForTest()}}
	event := domain.AvatarUploadEvent{AvatarID: id.String(), UserID: "user", S3Key: "original"}
	worker := New(repo, store, nil, 2, time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
	worker.sleep = func(context.Context, time.Duration) error { return nil }
	return worker, repo, store, event
}

var cachedPNG []byte

func pngDataForTest() []byte {
	if cachedPNG == nil {
		var buf bytes.Buffer
		_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 40, 20)))
		cachedPNG = buf.Bytes()
	}
	return cachedPNG
}

func TestProcessUpload(t *testing.T) {
	worker, repo, store, event := workerFixture()
	require.NoError(t, worker.Process(context.Background(), broker.Event{Type: broker.EventUpload, Upload: event}))
	require.Equal(t, []string{domain.ProcessingProcessing, domain.ProcessingCompleted}, repo.statuses)
	require.Len(t, store.objects, 3)
	require.Contains(t, store.objects, "thumbnails/"+event.AvatarID+"/100x100.jpg")
	require.Contains(t, store.objects, "thumbnails/"+event.AvatarID+"/300x300.jpg")
	downloads := store.downloads
	require.NoError(t, worker.Process(context.Background(), broker.Event{Type: broker.EventUpload, Upload: event}))
	require.Equal(t, downloads, store.downloads, "completed event must be idempotent")
}

func TestProcessUploadSkipsDeletedAndMissing(t *testing.T) {
	worker, repo, _, event := workerFixture()
	now := time.Now()
	repo.avatar.DeletedAt = &now
	require.NoError(t, worker.Process(context.Background(), broker.Event{Type: broker.EventUpload, Upload: event}))
	require.Empty(t, repo.statuses)
	repo.avatar = nil
	require.NoError(t, worker.Process(context.Background(), broker.Event{Type: broker.EventUpload, Upload: event}))
}

func TestProcessUploadErrors(t *testing.T) {
	worker, _, _, event := workerFixture()
	event.AvatarID = "bad"
	require.Error(t, worker.Process(context.Background(), broker.Event{Type: broker.EventUpload, Upload: event}))
	worker, _, store, event := workerFixture()
	store.err = errors.New("s3 down")
	require.Error(t, worker.Process(context.Background(), broker.Event{Type: broker.EventUpload, Upload: event}))
	require.Error(t, worker.Process(context.Background(), broker.Event{Type: "unknown"}))
}

func TestProcessDeleteIsIdempotent(t *testing.T) {
	worker, _, store, _ := workerFixture()
	event := domain.AvatarDeleteEvent{AvatarID: uuid.NewString(), S3Keys: []string{"original", "original", "", "thumb"}}
	require.NoError(t, worker.Process(context.Background(), broker.Event{Type: broker.EventDelete, Delete: event}))
	require.Equal(t, []string{"original", "thumb"}, store.deleted)
	require.NoError(t, worker.Process(context.Background(), broker.Event{Type: broker.EventDelete, Delete: event}))
}

func TestRetryAndFinalFailure(t *testing.T) {
	worker, repo, store, event := workerFixture()
	store.err = errors.New("temporary")
	sleeps := 0
	worker.sleep = func(context.Context, time.Duration) error { sleeps++; return nil }
	err := worker.processWithRetry(context.Background(), broker.Event{Type: broker.EventUpload, Upload: event})
	require.Error(t, err)
	require.Equal(t, 2, sleeps)
	require.Equal(t, domain.ProcessingFailed, repo.avatar.ProcessingStatus)
}

type consumerStub struct{ deliveries chan broker.Delivery }

func (c consumerStub) Consume(context.Context) (<-chan broker.Delivery, error) {
	return c.deliveries, nil
}
func (c consumerStub) Ping(context.Context) error { return nil }
func (c consumerStub) Close() error               { return nil }

func TestRunAcknowledgesDelivery(t *testing.T) {
	worker, _, _, event := workerFixture()
	deliveries := make(chan broker.Delivery, 1)
	acked := false
	deliveries <- broker.Delivery{Event: broker.Event{Type: broker.EventUpload, Upload: event}, Ack: func() error { acked = true; close(deliveries); return nil }, Reject: func(bool) error { return nil }}
	worker.consumer = consumerStub{deliveries: deliveries}
	require.Error(t, worker.Run(context.Background()))
	require.True(t, acked)
}
