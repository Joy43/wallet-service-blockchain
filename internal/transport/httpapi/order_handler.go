package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/yourorg/order-service/internal/domain"
)

type orderHandler struct {
	svc OrderService
	log *slog.Logger
}

// DTOs: the public JSON contract. Kept separate from domain types so
// the API can evolve without changing business code (and vice versa).
type createOrderRequest struct {
	CustomerID  string `json:"customer_id"`
	ProductID   string `json:"product_id"`
	Quantity    int    `json:"quantity"`
	AmountCents int64  `json:"amount_cents"`
}

type orderResponse struct {
	ID          string    `json:"id"`
	CustomerID  string    `json:"customer_id"`
	ProductID   string    `json:"product_id"`
	Quantity    int       `json:"quantity"`
	AmountCents int64     `json:"amount_cents"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

func toResponse(o domain.Order) orderResponse {
	return orderResponse{
		ID:          o.ID,
		CustomerID:  o.CustomerID,
		ProductID:   o.ProductID,
		Quantity:    o.Quantity,
		AmountCents: o.AmountCents,
		Status:      string(o.Status),
		CreatedAt:   o.CreatedAt,
	}
}

func (h *orderHandler) create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit

	var req createOrderRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		h.writeError(w, r, domain.NewError(domain.ErrInvalid, "invalid JSON body"))
		return
	}

	order, err := h.svc.CreateOrder(r.Context(), domain.CreateOrderInput{
		CustomerID:  req.CustomerID,
		ProductID:   req.ProductID,
		Quantity:    req.Quantity,
		AmountCents: req.AmountCents,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	w.Header().Set("Location", "/v1/orders/"+order.ID)
	writeJSON(w, http.StatusCreated, toResponse(order))
}

func (h *orderHandler) get(w http.ResponseWriter, r *http.Request) {
	order, err := h.svc.GetOrder(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toResponse(order))
}
