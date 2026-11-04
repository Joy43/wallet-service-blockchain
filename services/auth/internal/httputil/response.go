// Package httputil holds HTTP helpers shared by controllers and middleware:
// JSON responses, error mapping, strict body decoding and request-ID context.
// It exists so middleware and controller never import each other.
package httputil

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/yourorg/wallet-service/services/auth/internal/dto"
	"github.com/yourorg/wallet-service/services/auth/internal/model"
)

// StatusFor maps model error codes to HTTP status codes.
// This is the ONLY place that translation happens.
func StatusFor(code model.ErrorCode) int {
	switch code {
	case model.ErrInvalid:
		return http.StatusBadRequest
	case model.ErrNotFound:
		return http.StatusNotFound
	case model.ErrConflict:
		return http.StatusConflict
	case model.ErrUnauthenticated:
		return http.StatusUnauthorized
	case model.ErrForbidden:
		return http.StatusForbidden
	case model.ErrRateLimited:
		return http.StatusTooManyRequests
	case model.ErrUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// WriteError renders err as the standard JSON error body.
func WriteError(w http.ResponseWriter, r *http.Request, log *slog.Logger, service string, err error) {
	code := model.CodeOf(err)
	status := StatusFor(code)

	if status >= 500 {
		log.ErrorContext(r.Context(), "request failed",
			slog.String("request_id", RequestIDFrom(r.Context())),
			slog.Any("error", err))
	}

	WriteJSON(w, status, dto.ErrorResponse{
		Code:      string(code),
		Message:   model.MessageOf(err),
		Service:   service,
		RequestID: RequestIDFrom(r.Context()),
	})
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Decode reads a size-limited (1 MB), strict JSON body: unknown fields are rejected.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return model.NewError(model.ErrInvalid, "invalid JSON body")
	}
	return nil
}

type requestIDKey struct{}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
