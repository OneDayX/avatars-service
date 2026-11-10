package observability

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type StatusWriter struct {
	http.ResponseWriter
	status int
}

func (w *StatusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *StatusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func HTTPTracing(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isOperationalPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			ctx, span := otel.Tracer(serviceName+"/http").Start(ctx, r.Method+" "+r.URL.Path,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("http.request.method", r.Method),
					attribute.String("url.path", r.URL.Path),
					attribute.String("network.protocol.version", r.Proto),
				),
			)
			defer span.End()
			writer := &StatusWriter{ResponseWriter: w}
			next.ServeHTTP(writer, r.WithContext(ctx))
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route != "" {
				span.SetName(r.Method + " " + route)
				span.SetAttributes(attribute.String("http.route", route))
			}
			status := writer.status
			if status == 0 {
				status = http.StatusOK
			}
			span.SetAttributes(attribute.Int("http.response.status_code", status))
			if status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, "HTTP server error")
			}
		})
	}
}
