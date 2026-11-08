package observability

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

func TestHTTPMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	gatherer := prometheus.Gatherer(registry)
	_ = gatherer
	router := chi.NewRouter()
	router.Use(HTTPMetrics(slog.New(slog.NewTextHandler(testDiscard{}, nil))))
	router.Get("/metrics-test", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics-test", nil))
	require.Equal(t, http.StatusTeapot, response.Code)
	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	found := false
	for _, family := range families {
		if family.GetName() == "avatars_http_requests_total" {
			found = true
		}
	}
	require.True(t, found)
}

type testDiscard struct{}

func (testDiscard) Write(data []byte) (int, error) { return len(data), nil }
