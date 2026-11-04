package controller

import (
	"context"
	"log/slog"
	"net/http"
)

// HealthController serves liveness and readiness probes (used by Kubernetes).
type HealthController struct {
	ready func(context.Context) error
	log   *slog.Logger
}

func NewHealthController(ready func(context.Context) error, log *slog.Logger) *HealthController {
	return &HealthController{ready: ready, log: log}
}

// Live: the process is up.
func (h *HealthController) Live(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// Ready: dependencies (database, Redis, NATS) are reachable.
func (h *HealthController) Ready(w http.ResponseWriter, r *http.Request) {
	if err := h.ready(r.Context()); err != nil {
		h.log.WarnContext(r.Context(), "not ready", slog.Any("error", err))
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}
