// Package domain holds pure business types and rules.
// RULE: imports only the standard library — never another package of this service.
package domain

import "time"

type OrderStatus string

const (
	StatusPending   OrderStatus = "PENDING"
	StatusConfirmed OrderStatus = "CONFIRMED"
	StatusCancelled OrderStatus = "CANCELLED"
)

type Order struct {
	ID          string
	CustomerID  string
	ProductID   string
	Quantity    int
	AmountCents int64
	Status      OrderStatus
	CreatedAt   time.Time
}

// CreateOrderInput is what the business layer needs to create an order,
// independent of whether it arrived via HTTP, gRPC or a message.
type CreateOrderInput struct {
	CustomerID  string
	ProductID   string
	Quantity    int
	AmountCents int64
}

func (in CreateOrderInput) Validate() error {
	switch {
	case in.CustomerID == "":
		return NewError(ErrInvalid, "customer_id is required")
	case in.ProductID == "":
		return NewError(ErrInvalid, "product_id is required")
	case in.Quantity <= 0:
		return NewError(ErrInvalid, "quantity must be greater than 0")
	case in.AmountCents <= 0:
		return NewError(ErrInvalid, "amount_cents must be greater than 0")
	}
	return nil
}
