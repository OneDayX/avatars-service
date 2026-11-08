package handlers

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHealthHandlerLive(t *testing.T) {
	handler := NewHealthHandler(nil, nil, nil, slog.Default())
	response := httptest.NewRecorder()
	handler.Live(response, httptest.NewRequest(http.MethodGet, "/live", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"status":"ok"}`, response.Body.String())
}
