package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/yourorg/wallet-service/services/auth/internal/httputil"
	"github.com/yourorg/wallet-service/services/auth/internal/model"
)

// TokenValidator is satisfied both by the in-process AuthService (auth-service)
// and by the gRPC client (other services), so the middleware is shared.
type TokenValidator interface {
	ValidateToken(ctx context.Context, token string) (model.Claims, error)
}

type claimsKey struct{}

// ClaimsFrom returns the authenticated caller, set by Authenticate.
func ClaimsFrom(ctx context.Context) (model.Claims, bool) {
	c, ok := ctx.Value(claimsKey{}).(model.Claims)
	return c, ok
}

func Authenticate(v TokenValidator, log *slog.Logger, service string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				w.Header().Set("WWW-Authenticate", "Bearer")
				httputil.WriteError(w, r, log, service, model.NewError(model.ErrUnauthenticated, "missing bearer token"))
				return
			}
			claims, err := v.ValidateToken(r.Context(), token)
			if err != nil {
				if model.CodeOf(err) == model.ErrUnauthenticated {
					w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
				}
				httputil.WriteError(w, r, log, service, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey{}, claims)))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return "", false
	}
	return strings.TrimSpace(token), true
}

// RequireRole allows only callers whose token carries one of the given roles.
// Must run after Authenticate. Unauthenticated callers get 401, wrong role gets 403.
func RequireRole(log *slog.Logger, service string, roles ...string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, ok := ClaimsFrom(r.Context())
			if !ok {
				httputil.WriteError(w, r, log, service, model.NewError(model.ErrUnauthenticated, "authentication required"))
				return
			}
			for _, role := range roles {
				if c.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			httputil.WriteError(w, r, log, service, model.NewError(model.ErrForbidden, "insufficient permissions"))
		})
	}
}
