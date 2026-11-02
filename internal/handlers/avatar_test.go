package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/example/gophprofile/internal/domain"
	"github.com/example/gophprofile/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type stubService struct {
	avatar  *domain.Avatar
	err     error
	userID  string
	deleted bool
}

func (s *stubService) Upload(_ context.Context, userID, _ string, _ []byte) (*domain.Avatar, error) {
	s.userID = userID
	return s.avatar, s.err
}
func (s *stubService) Get(context.Context, uuid.UUID, string) (*domain.Avatar, storage.Object, error) {
	if s.err != nil {
		return nil, storage.Object{}, s.err
	}
	return s.avatar, storage.Object{Body: io.NopCloser(strings.NewReader("image")), ContentType: "image/png", Size: 5}, nil
}
func (s *stubService) GetLatest(context.Context, string, string) (*domain.Avatar, storage.Object, error) {
	return s.Get(context.Background(), uuid.Nil, "")
}
func (s *stubService) Metadata(context.Context, uuid.UUID) (*domain.Avatar, error) {
	return s.avatar, s.err
}
func (s *stubService) List(context.Context, string) ([]domain.Avatar, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []domain.Avatar{*s.avatar}, nil
}
func (s *stubService) Delete(context.Context, uuid.UUID, string) error { return s.err }
func (s *stubService) DeleteLatest(_ context.Context, userID string) error {
	s.deleted = true
	return s.err
}

func testAvatar() *domain.Avatar {
	now := time.Now().UTC()
	return &domain.Avatar{ID: uuid.New(), UserID: "user", FileName: "me.png", MIMEType: "image/png", ProcessingStatus: "pending", ThumbnailS3Keys: map[string]string{}, CreatedAt: now, UpdatedAt: now}
}

func multipartRequest(t *testing.T, data []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "avatar.png")
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/avatars", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-User-ID", "user")
	return req
}

func TestUploadHandler(t *testing.T) {
	service := &stubService{avatar: testAvatar()}
	handler := NewAvatarHandler(service, 1024)
	response := httptest.NewRecorder()
	handler.Upload(response, multipartRequest(t, []byte("png")))
	require.Equal(t, http.StatusCreated, response.Code)
	require.Equal(t, "user", service.userID)
	require.Contains(t, response.Body.String(), `"status":"processing"`)
}

func TestUploadHandlerErrors(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{{domain.ErrInvalidFile, 400}, {domain.ErrFileTooLarge, 413}, {domain.ErrInvalidUserID, 400}, {errors.New("db"), 500}} {
		service := &stubService{err: test.err}
		handler := NewAvatarHandler(service, 1024)
		response := httptest.NewRecorder()
		handler.Upload(response, multipartRequest(t, []byte("data")))
		require.Equal(t, test.status, response.Code)
	}
	handler := NewAvatarHandler(&stubService{}, 2)
	response := httptest.NewRecorder()
	handler.Upload(response, multipartRequest(t, []byte("large")))
	require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
}

func avatarRouter(service AvatarService) http.Handler {
	handler := NewAvatarHandler(service, 1024)
	router := chi.NewRouter()
	router.Get("/api/v1/avatars/{avatar_id}", handler.Get)
	router.Get("/api/v1/avatars/{avatar_id}/metadata", handler.Metadata)
	router.Delete("/api/v1/avatars/{avatar_id}", handler.Delete)
	router.Get("/api/v1/users/{user_id}/avatars", handler.List)
	router.Get("/api/v1/users/{user_id}/avatar", handler.GetLatest)
	router.Delete("/api/v1/users/{user_id}/avatar", handler.DeleteLatest)
	return router
}

func TestGetImageAndETag(t *testing.T) {
	avatar := testAvatar()
	router := avatarRouter(&stubService{avatar: avatar})
	url := "/api/v1/avatars/" + avatar.ID.String()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, url, nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "image", response.Body.String())
	require.Equal(t, "image/png", response.Header().Get("Content-Type"))
	require.NotEmpty(t, response.Header().Get("ETag"))
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("If-None-Match", response.Header().Get("ETag"))
	cached := httptest.NewRecorder()
	router.ServeHTTP(cached, req)
	require.Equal(t, http.StatusNotModified, cached.Code)
}

func TestReadHandlerErrors(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{{domain.ErrNotFound, 404}, {domain.ErrNotReady, 409}, {domain.ErrInvalidSize, 400}, {errors.New("s3"), 500}} {
		router := avatarRouter(&stubService{avatar: testAvatar(), err: test.err})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/avatars/"+uuid.NewString(), nil))
		require.Equal(t, test.status, response.Code)
	}
	response := httptest.NewRecorder()
	avatarRouter(&stubService{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/avatars/not-a-uuid", nil))
	require.Equal(t, http.StatusBadRequest, response.Code)
}

func TestMetadataAndListHandlers(t *testing.T) {
	avatar := testAvatar()
	router := avatarRouter(&stubService{avatar: avatar})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/avatars/"+avatar.ID.String()+"/metadata", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"file_name":"me.png"`)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/users/user/avatars", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), avatar.ID.String())
}

func TestDeleteHandlers(t *testing.T) {
	avatar := testAvatar()
	service := &stubService{avatar: avatar}
	router := avatarRouter(service)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/avatars/"+avatar.ID.String(), nil))
	require.Equal(t, http.StatusNoContent, response.Code)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/users/user/avatar", nil))
	require.Equal(t, http.StatusForbidden, response.Code)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/user/avatar", nil)
	req.Header.Set("X-User-ID", "user")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, req)
	require.Equal(t, http.StatusNoContent, response.Code)
	require.True(t, service.deleted)
}

func TestHealthHandler(t *testing.T) {
	healthy := &stubService{}
	handler := NewHealthHandler(healthyPinger{}, healthyPinger{}, healthyPinger{})
	response := httptest.NewRecorder()
	handler.Check(response, httptest.NewRequest(http.MethodGet, "/health", nil))
	require.Equal(t, http.StatusOK, response.Code)
	handler = NewHealthHandler(healthyPinger{}, healthyPinger{}, healthyPinger{err: errors.New("down")})
	response = httptest.NewRecorder()
	handler.Check(response, httptest.NewRequest(http.MethodGet, "/health", nil))
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, response.Body.String(), "degraded")
	_ = healthy
}

type healthyPinger struct{ err error }

func (p healthyPinger) Ping(context.Context) error { return p.err }

func TestRateLimiterAndCORS(t *testing.T) {
	limiter := NewRateLimiter(1, time.Minute)
	now := time.Now()
	limiter.now = func() time.Time { return now }
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := CORS("*")(limiter.Middleware(next))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	require.Equal(t, http.StatusNoContent, response.Code)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	require.Equal(t, http.StatusTooManyRequests, response.Code)
	now = now.Add(time.Minute)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	require.Equal(t, http.StatusNoContent, response.Code)
	req = httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	require.Equal(t, http.StatusNoContent, response.Code)
	require.Equal(t, "*", response.Header().Get("Access-Control-Allow-Origin"))
}

func TestWebHandlers(t *testing.T) {
	avatar := testAvatar()
	service := &stubService{avatar: avatar}
	handler, err := NewWebHandler(service, "../../web", 1024)
	require.NoError(t, err)
	response := httptest.NewRecorder()
	handler.UploadForm(response, httptest.NewRequest(http.MethodGet, "/web/upload", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), "GophProfile")
	service.err = domain.ErrInvalidUserID
	response = httptest.NewRecorder()
	handler.Upload(response, multipartRequest(t, []byte("png")))
	require.Equal(t, http.StatusBadRequest, response.Code)
	service.err = nil
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("user_id", "user"))
	part, err := writer.CreateFormFile("file", "a.png")
	require.NoError(t, err)
	_, _ = part.Write([]byte("png"))
	require.NoError(t, writer.Close())
	req := httptest.NewRequest(http.MethodPost, "/web/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response = httptest.NewRecorder()
	handler.Upload(response, req)
	require.Equal(t, http.StatusSeeOther, response.Code)
	router := chi.NewRouter()
	router.Get("/web/gallery/{user_id}", handler.Gallery)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/web/gallery/user", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), avatar.ID.String())
}
