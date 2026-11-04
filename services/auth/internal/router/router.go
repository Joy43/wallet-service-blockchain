// Package router maps URLs to controllers and assembles the middleware chain.
// It is the only place that knows which route is public and which is protected.
package router

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/yourorg/wallet-service/services/auth/internal/controller"
	"github.com/yourorg/wallet-service/services/auth/internal/middleware"
)

// AuthService is everything auth-service's router needs from the service layer.
type AuthService interface {
	controller.AuthService
	middleware.TokenValidator
}

func NewAuthRouter(svc AuthService, log *slog.Logger, ready func(context.Context) error) http.Handler {
	auth := controller.NewAuthController(svc, log)
	authed := middleware.Authenticate(svc, log, "auth-service")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/jwks.json", auth.JWKS)
	mux.HandleFunc("POST /v1/auth/register", auth.Register)
	mux.HandleFunc("POST /v1/auth/login", auth.Login)
	mux.HandleFunc("POST /v1/auth/refresh", auth.Refresh)
	mux.Handle("POST /v1/auth/logout", authed(http.HandlerFunc(auth.Logout)))
	mux.Handle("GET /v1/auth/me", authed(http.HandlerFunc(auth.Me)))

	mountOps(mux, log, ready)
	return withCommonMiddleware(mux, log)
}

// mountOps registers health probes, metrics and API docs.
func mountOps(mux *http.ServeMux, log *slog.Logger, ready func(context.Context) error) {
	health := controller.NewHealthController(ready, log)
	mux.HandleFunc("GET /healthz", health.Live)
	mux.HandleFunc("GET /readyz", health.Ready)
	mux.Handle("GET /metrics", promhttp.Handler())
	registerDocs(mux)
}

// Middleware runs outermost-first: requestID -> recover -> log -> metrics -> mux
func withCommonMiddleware(h http.Handler, log *slog.Logger) http.Handler {
	return middleware.Chain(h,
		middleware.RequestID,
		middleware.Recoverer(log),
		middleware.RequestLogger(log),
		middleware.Metrics,
	)
}
