package middleware

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yourorg/wallet-service/services/auth/internal/model"
)

type stubValidator struct{ err error }

func (s stubValidator) ValidateToken(_ context.Context, tok string) (model.Claims, error) {
	if s.err != nil {
		return model.Claims{}, s.err
	}
	return model.Claims{UserID: "u-" + tok}, nil
}

func TestRequireAuth(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := ClaimsFrom(r.Context())
		_, _ = io.WriteString(w, c.UserID)
	})

	tests := []struct {
		name   string
		header string
		v      stubValidator
		status int
		body   string
	}{
		{"no header", "", stubValidator{}, 401, ""},
		{"wrong scheme", "Basic abc", stubValidator{}, 401, ""},
		{"empty bearer", "Bearer ", stubValidator{}, 401, ""},
		{"valid", "Bearer abc", stubValidator{}, 200, "u-abc"},
		{"case-insensitive scheme", "bearer abc", stubValidator{}, 200, "u-abc"},
		{"rejected", "Bearer abc", stubValidator{err: model.NewError(model.ErrUnauthenticated, "x")}, 401, ""},
		{"auth backend down fails closed", "Bearer abc", stubValidator{err: model.NewError(model.ErrUnavailable, "x")}, 503, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			Authenticate(tt.v, log, "test")(next).ServeHTTP(rec, req)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}
			if tt.body != "" && rec.Body.String() != tt.body {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.body)
			}
		})
	}
}

func TestRequireRole(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	guard := RequireRole(log, "test", model.RoleAdmin)(ok)

	tests := []struct {
		name   string
		claims *model.Claims
		status int
	}{
		{"no claims", nil, 401},
		{"wrong role", &model.Claims{UserID: "u", Role: model.RoleUser}, 403},
		{"allowed role", &model.Claims{UserID: "u", Role: model.RoleAdmin}, 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.claims != nil {
				req = req.WithContext(context.WithValue(req.Context(), claimsKey{}, *tt.claims))
			}
			rec := httptest.NewRecorder()
			guard.ServeHTTP(rec, req)
			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d", rec.Code, tt.status)
			}
		})
	}
}
