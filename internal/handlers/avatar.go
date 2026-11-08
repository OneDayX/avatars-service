package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/example/gophprofile/internal/domain"
	"github.com/example/gophprofile/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type AvatarService interface {
	Upload(ctx context.Context, userID, fileName string, data []byte) (*domain.Avatar, error)
	Get(ctx context.Context, id uuid.UUID, size string) (*domain.Avatar, storage.Object, error)
	GetLatest(ctx context.Context, userID, size string) (*domain.Avatar, storage.Object, error)
	Metadata(ctx context.Context, id uuid.UUID) (*domain.Avatar, error)
	List(ctx context.Context, userID string) ([]domain.Avatar, error)
	Delete(ctx context.Context, id uuid.UUID, userID string) error
	DeleteLatest(ctx context.Context, userID string) error
}

type AvatarHandler struct {
	service     AvatarService
	maxFileSize int64
	logger      *slog.Logger
}

func NewAvatarHandler(service AvatarService, maxFileSize int64, logger *slog.Logger) *AvatarHandler {
	return &AvatarHandler{service: service, maxFileSize: maxFileSize, logger: logger}
}

func (h *AvatarHandler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxFileSize+(1<<20))
	if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
		h.uploadError(r.Context(), w, err)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid file", "multipart field file is required")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, h.maxFileSize+1))
	if err != nil {
		h.uploadError(r.Context(), w, err)
		return
	}
	if int64(len(data)) > h.maxFileSize {
		writeFileTooLarge(w, h.maxFileSize)
		return
	}
	avatar, err := h.service.Upload(r.Context(), r.Header.Get("X-User-ID"), header.Filename, data)
	if err != nil {
		h.uploadError(r.Context(), w, err)
		return
	}
	writeJSON(w, http.StatusCreated, avatar.UploadResponse())
}

func (h *AvatarHandler) uploadError(ctx context.Context, w http.ResponseWriter, err error) {
	var maxBytesError *http.MaxBytesError
	switch {
	case errors.As(err, &maxBytesError), errors.Is(err, domain.ErrFileTooLarge):
		writeFileTooLarge(w, h.maxFileSize)
	case errors.Is(err, domain.ErrInvalidFile):
		writeError(w, http.StatusBadRequest, "Invalid file format", "Supported formats: jpeg, png, webp; file content must be a valid image")
	case errors.Is(err, domain.ErrInvalidUserID):
		writeError(w, http.StatusBadRequest, "Invalid User-ID", "X-User-ID must be a non-empty string up to 255 bytes")
	default:
		h.internalError(ctx, w, err)
	}
}

func (h *AvatarHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "avatar_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid avatar ID", "Avatar ID must be a UUID")
		return
	}
	avatar, object, err := h.service.Get(r.Context(), id, r.URL.Query().Get("size"))
	if err != nil {
		h.readError(r.Context(), w, err)
		return
	}
	defer func() { _ = object.Body.Close() }()
	serveImage(w, r, avatar, object)
}

func (h *AvatarHandler) GetLatest(w http.ResponseWriter, r *http.Request) {
	avatar, object, err := h.service.GetLatest(r.Context(), chi.URLParam(r, "user_id"), r.URL.Query().Get("size"))
	if err != nil {
		h.readError(r.Context(), w, err)
		return
	}
	defer func() { _ = object.Body.Close() }()
	serveImage(w, r, avatar, object)
}

func (h *AvatarHandler) readError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "Avatar not found", "")
	case errors.Is(err, domain.ErrNotReady):
		writeError(w, http.StatusConflict, "Avatar not ready", "Thumbnails have not been generated yet")
	case errors.Is(err, domain.ErrInvalidSize):
		writeError(w, http.StatusBadRequest, "Invalid size", "Supported sizes: 100x100, 300x300, original")
	case errors.Is(err, domain.ErrInvalidUserID):
		writeError(w, http.StatusBadRequest, "Invalid User-ID", "User ID must be valid")
	default:
		h.internalError(ctx, w, err)
	}
}

func serveImage(w http.ResponseWriter, r *http.Request, avatar *domain.Avatar, object storage.Object) {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%s", avatar.ID, avatar.SizeBytes, avatar.UpdatedAt.UnixNano(), r.URL.Query().Get("size"))))
	etag := `"` + hex.EncodeToString(hash[:]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", object.ContentType)
	if object.Size > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", object.Size))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, object.Body)
}

func (h *AvatarHandler) Metadata(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "avatar_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid avatar ID", "Avatar ID must be a UUID")
		return
	}
	avatar, err := h.service.Metadata(r.Context(), id)
	if err != nil {
		h.readError(r.Context(), w, err)
		return
	}
	writeJSON(w, http.StatusOK, avatar.MetadataResponse())
}

type avatarListItem struct {
	ID         uuid.UUID         `json:"id"`
	UserID     string            `json:"user_id"`
	URL        string            `json:"url"`
	Status     string            `json:"status"`
	Dimensions domain.Dimensions `json:"dimensions"`
	CreatedAt  string            `json:"created_at"`
}

func (h *AvatarHandler) List(w http.ResponseWriter, r *http.Request) {
	avatars, err := h.service.List(r.Context(), chi.URLParam(r, "user_id"))
	if err != nil {
		h.readError(r.Context(), w, err)
		return
	}
	items := make([]avatarListItem, 0, len(avatars))
	for _, avatar := range avatars {
		items = append(items, avatarListItem{
			ID: avatar.ID, UserID: avatar.UserID, URL: avatar.URL(), Status: avatar.ProcessingStatus,
			Dimensions: avatar.Dimensions, CreatedAt: avatar.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *AvatarHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "avatar_id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid avatar ID", "Avatar ID must be a UUID")
		return
	}
	if err := h.service.Delete(r.Context(), id, r.Header.Get("X-User-ID")); err != nil {
		h.deleteError(r.Context(), w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AvatarHandler) DeleteLatest(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")
	if r.Header.Get("X-User-ID") != userID {
		writeError(w, http.StatusForbidden, "Forbidden", "You can only delete your own avatars")
		return
	}
	if err := h.service.DeleteLatest(r.Context(), userID); err != nil {
		h.deleteError(r.Context(), w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AvatarHandler) deleteError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "Avatar not found", "")
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, "Forbidden", "You can only delete your own avatars")
	case errors.Is(err, domain.ErrInvalidUserID):
		writeError(w, http.StatusBadRequest, "Invalid User-ID", "X-User-ID is required")
	default:
		h.internalError(ctx, w, err)
	}
}

func (h *AvatarHandler) internalError(ctx context.Context, w http.ResponseWriter, err error) {
	h.logger.ErrorContext(ctx, "avatar request failed", "error", err)
	writeError(w, http.StatusInternalServerError, "Internal server error", "")
}

func writeFileTooLarge(w http.ResponseWriter, maxSize int64) {
	writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "File too large", "max_size": maxSize})
}

func writeError(w http.ResponseWriter, status int, message, details string) {
	body := map[string]any{"error": message}
	if details != "" {
		body["details"] = details
	}
	writeJSON(w, status, body)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
