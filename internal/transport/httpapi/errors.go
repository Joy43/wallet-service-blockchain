package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/yourorg/order-service/internal/domain"
)

type errorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Service   string `json:"service"`
	RequestID string `json:"request_id,omitempty"`
}

// statusFor maps domain error codes to HTTP status codes.
// This is the ONLY place that translation happens.
func statusFor(code domain.ErrorCode) int {
	switch code {
	case domain.ErrInvalid:
		return http.StatusBadRequest
	case domain.ErrNotFound:
		return http.StatusNotFound
	case domain.ErrConflict:
		return http.StatusConflict
	case domain.ErrUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func (h *orderHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	code := domain.CodeOf(err)
	status := statusFor(code)

	if status >= 500 {
		h.log.ErrorContext(r.Context(), "request failed",
			slog.String("request_id", requestIDFrom(r.Context())),
			slog.Any("error", err))
	}

	writeJSON(w, status, errorResponse{
		Code:      string(code),
		Message:   domain.MessageOf(err),
		Service:   "order-service",
		RequestID: requestIDFrom(r.Context()),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
