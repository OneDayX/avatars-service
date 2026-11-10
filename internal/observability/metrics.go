package observability

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/example/gophprofile/internal/metrics"
	"github.com/go-chi/chi/v5"
)

func HTTPMetrics(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isOperationalPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			started := time.Now()
			writer := &StatusWriter{ResponseWriter: w}
			next.ServeHTTP(writer, r)
			status := writer.status
			if status == 0 {
				status = http.StatusOK
			}
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unmatched"
			}
			metrics.HTTPRequests.WithLabelValues(r.Method, route, strconv.Itoa(status)).Inc()
			metrics.HTTPDuration.WithLabelValues(r.Method, route).Observe(time.Since(started).Seconds())
			if status >= http.StatusInternalServerError {
				metrics.HTTPErrors.WithLabelValues(r.Method, route).Inc()
			}
			logger.Log(r.Context(), slog.LevelInfo, "HTTP request",
				"method", r.Method, "path", route, "status", status, "duration_ms", time.Since(started).Milliseconds())
		})
	}
}
