package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type healthyPinger struct{ err error }

func (p healthyPinger) Ping(context.Context) error { return p.err }

func TestHealthHandlerLive(t *testing.T) {
	handler := NewHealthHandler(nil, nil, nil, slog.Default())
	response := httptest.NewRecorder()
	handler.Live(response, httptest.NewRequest(http.MethodGet, "/live", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"status":"ok"}`, response.Body.String())
}

func TestHealthHandlerReady(t *testing.T) {
	available := healthyPinger{}
	unavailable := healthyPinger{err: errors.New("down")}
	tests := []struct {
		name     string
		database Pinger
		storage  Pinger
		broker   Pinger
		status   int
		body     string
	}{
		{
			name: "all dependencies available", database: available, storage: available, broker: available,
			status: http.StatusOK,
			body:   `{"status":"ok","components":{"database":"ok","s3":"ok","broker":"ok"}}`,
		},
		{
			name: "database unavailable", database: unavailable, storage: available, broker: available,
			status: http.StatusServiceUnavailable,
			body:   `{"status":"degraded","components":{"database":"error","s3":"ok","broker":"ok"}}`,
		},
		{
			name: "storage unavailable", database: available, storage: unavailable, broker: available,
			status: http.StatusServiceUnavailable,
			body:   `{"status":"degraded","components":{"database":"ok","s3":"error","broker":"ok"}}`,
		},
		{
			name: "broker unavailable", database: available, storage: available, broker: unavailable,
			status: http.StatusServiceUnavailable,
			body:   `{"status":"degraded","components":{"database":"ok","s3":"ok","broker":"error"}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := NewHealthHandler(test.database, test.storage, test.broker, testLogger())
			response := httptest.NewRecorder()
			handler.Ready(response, httptest.NewRequest(http.MethodGet, "/ready", nil))
			require.Equal(t, test.status, response.Code)
			require.JSONEq(t, test.body, response.Body.String())
			require.NotContains(t, response.Body.String(), "down")
		})
	}
}
