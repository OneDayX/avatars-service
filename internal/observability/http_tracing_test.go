package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/attribute"
)

func TestHTTPTracing(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousTracer := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTracerProvider(previousTracer); otel.SetTextMapPropagator(previousPropagator) })
	router := chi.NewRouter()
	router.Use(HTTPTracing("test"))
	router.Get("/users/{user_id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusCreated) })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/users/42", nil))
	require.Equal(t, http.StatusCreated, response.Code)
	spans := recorder.Ended()
	require.Len(t, spans, 1)
	require.Equal(t, "GET /users/{user_id}", spans[0].Name())
	require.Equal(t, int(http.StatusCreated), intAttribute(spans[0].Attributes(), "http.response.status_code"))
}

func intAttribute(attributes []attribute.KeyValue, key attribute.Key) int {
	for _, item := range attributes {
		if item.Key == key {
			return int(item.Value.AsInt64())
		}
	}
	return 0
}

func TestStatusWriterDefaultsToOK(t *testing.T) {
	response := httptest.NewRecorder()
	writer := &StatusWriter{ResponseWriter: response}
	_, err := writer.Write([]byte("ok"))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.Code)
	writer.WriteHeader(http.StatusInternalServerError)
	require.Equal(t, http.StatusOK, response.Code)
	_ = context.Background()
}
