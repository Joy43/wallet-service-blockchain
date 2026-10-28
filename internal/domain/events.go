package domain

import "time"

// OrderCreated is published when a new order is stored.
// Inventory and billing services consume it asynchronously.
type OrderCreated struct {
	OrderID     string    `json:"order_id"`
	CustomerID  string    `json:"customer_id"`
	ProductID   string    `json:"product_id"`
	Quantity    int       `json:"quantity"`
	AmountCents int64     `json:"amount_cents"`
	OccurredAt  time.Time `json:"occurred_at"`
}
