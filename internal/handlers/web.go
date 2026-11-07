package handlers

import (
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"
)

type WebHandler struct {
	service     AvatarService
	templates   *template.Template
	maxFileSize int64
	logger      *slog.Logger
}

func NewWebHandler(service AvatarService, webDirectory string, maxFileSize int64, logger *slog.Logger) (*WebHandler, error) {
	templates, err := template.ParseGlob(filepath.Join(webDirectory, "templates", "*.html"))
	if err != nil {
		return nil, err
	}
	return &WebHandler{service: service, templates: templates, maxFileSize: maxFileSize, logger: logger}, nil
}

func (h *WebHandler) UploadForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, http.StatusOK, "upload.html", map[string]any{"UserID": r.URL.Query().Get("user_id")})
}

func (h *WebHandler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxFileSize+(1<<20))
	if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
		h.render(w, http.StatusBadRequest, "upload.html", map[string]any{"Error": "Не удалось прочитать форму или файл слишком большой"})
		return
	}
	userID := r.FormValue("user_id")
	file, header, err := r.FormFile("file")
	if err != nil {
		h.render(w, http.StatusBadRequest, "upload.html", map[string]any{"Error": "Выберите изображение", "UserID": userID})
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, h.maxFileSize+1))
	if err == nil && int64(len(data)) <= h.maxFileSize {
		_, err = h.service.Upload(r.Context(), userID, header.Filename, data)
	}
	if err != nil || int64(len(data)) > h.maxFileSize {
		h.render(w, http.StatusBadRequest, "upload.html", map[string]any{"Error": "Файл должен быть JPEG, PNG или WebP размером до 10 МБ", "UserID": userID})
		return
	}
	http.Redirect(w, r, "/web/gallery/"+userID, http.StatusSeeOther)
}

func (h *WebHandler) Gallery(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "user_id")
	avatars, err := h.service.List(r.Context(), userID)
	if err != nil {
		h.readError(w, err)
		return
	}
	h.render(w, http.StatusOK, "gallery.html", map[string]any{"UserID": userID, "Avatars": avatars})
}

func (h *WebHandler) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := h.templates.ExecuteTemplate(w, name, data); err != nil {
		return
	}
}

func (h *WebHandler) readError(w http.ResponseWriter, err error) {
	h.logger.Error("web request failed", "error", err)
	writeError(w, http.StatusInternalServerError, "Internal server error", "")
}
