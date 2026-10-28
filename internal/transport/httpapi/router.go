// Package httpapi is the INBOUND ADAPTER for HTTP.
// It is the only layer that knows about HTTP, JSON and status codes.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/yourorg/order-service/internal/domain"
)

// OrderService is defined where it is USED (consumer-side interface).
type OrderService interface {
	CreateOrder(ctx context.Context, in domain.CreateOrderInput) (domain.Order, error)
	GetOrder(ctx context.Context, id string) (domain.Order, error)
}

func NewRouter(svc OrderService, log *slog.Logger, ready func(context.Context) error) http.Handler {
	h := &orderHandler{svc: svc, log: log}

	mux := http.NewServeMux()

	// Business endpoints (Go 1.22+ method + path patterns)
	mux.HandleFunc("POST /v1/orders", h.create)
	mux.HandleFunc("GET /v1/orders/{id}", h.get)

	// Operational endpoints
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK) // liveness: process is up
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := ready(r.Context()); err != nil { // readiness: dependencies reachable
			log.WarnContext(r.Context(), "not ready", slog.Any("error", err))
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("GET /metrics", promhttp.Handler())

	// Middleware runs outermost-first: requestID -> recover -> log -> metrics -> mux
	return chain(mux,
		requestID,
		recoverer(log),
		requestLogger(log),
		metrics,
	)
}
