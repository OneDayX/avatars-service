package handlers

import (
	"context"
	"log/slog"
	"net/http"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type HealthHandler struct {
	database Pinger
	storage  Pinger
	broker   Pinger
	logger   *slog.Logger
}

func NewHealthHandler(database, storage, broker Pinger, logger *slog.Logger) *HealthHandler {
	return &HealthHandler{database: database, storage: storage, broker: broker, logger: logger}
}

func (h *HealthHandler) Check(w http.ResponseWriter, r *http.Request) {
	components := map[string]string{"database": "ok", "s3": "ok", "broker": "ok"}
	healthy := true
	for name, check := range map[string]func(context.Context) error{
		"database": h.database.Ping, "s3": h.storage.Ping, "broker": h.broker.Ping,
	} {
		if err := check(r.Context()); err != nil {
			h.logger.Error("health check failed", "component", name, "error", err)
			components[name] = "error"
			healthy = false
		}
	}
	status := http.StatusOK
	state := "ok"
	if !healthy {
		status = http.StatusServiceUnavailable
		state = "degraded"
	}
	writeJSON(w, status, map[string]any{"status": state, "components": components})
}
