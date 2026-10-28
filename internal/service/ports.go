package service

import (
	"context"

	"github.com/yourorg/order-service/internal/domain"
)

// Ports: the service depends ONLY on these interfaces.
// Postgres, NATS, or test fakes plug in from the outside (hexagonal architecture).

type OrderRepository interface {
	Create(ctx context.Context, o domain.Order) error
	GetByID(ctx context.Context, id string) (domain.Order, error)
}

type EventPublisher interface {
	PublishOrderCreated(ctx context.Context, e domain.OrderCreated) error
}
