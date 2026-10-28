// Package service contains the business use cases.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/yourorg/order-service/internal/domain"
)

type OrderService struct {
	repo   OrderRepository
	events EventPublisher
	log    *slog.Logger
	newID  func() string
	now    func() time.Time
}

func NewOrderService(repo OrderRepository, events EventPublisher, log *slog.Logger) *OrderService {
	return &OrderService{
		repo:   repo,
		events: events,
		log:    log,
		newID:  randomID,
		now:    time.Now,
	}
}

// CreateOrder stores a pending order and publishes OrderCreated.
// It does NOT call inventory or billing synchronously — they react to the event.
func (s *OrderService) CreateOrder(ctx context.Context, in domain.CreateOrderInput) (domain.Order, error) {
	if err := in.Validate(); err != nil {
		return domain.Order{}, err
	}

	order := domain.Order{
		ID:          s.newID(),
		CustomerID:  in.CustomerID,
		ProductID:   in.ProductID,
		Quantity:    in.Quantity,
		AmountCents: in.AmountCents,
		Status:      domain.StatusPending,
		CreatedAt:   s.now().UTC(),
	}

	if err := s.repo.Create(ctx, order); err != nil {
		return domain.Order{}, fmt.Errorf("create order: %w", err)
	}

	event := domain.OrderCreated{
		OrderID:     order.ID,
		CustomerID:  order.CustomerID,
		ProductID:   order.ProductID,
		Quantity:    order.Quantity,
		AmountCents: order.AmountCents,
		OccurredAt:  order.CreatedAt,
	}
	if err := s.events.PublishOrderCreated(ctx, event); err != nil {
		// The order is already saved, so we don't fail the request.
		// PRODUCTION NOTE: replace this with the transactional outbox pattern
		// (write the event to an outbox table in the same DB transaction,
		// and a relay publishes it) so events can never be lost.
		s.log.ErrorContext(ctx, "publish OrderCreated failed",
			slog.String("order_id", order.ID), slog.Any("error", err))
	}

	s.log.InfoContext(ctx, "order created",
		slog.String("order_id", order.ID),
		slog.String("customer_id", order.CustomerID))

	return order, nil
}

func (s *OrderService) GetOrder(ctx context.Context, id string) (domain.Order, error) {
	if id == "" {
		return domain.Order{}, domain.NewError(domain.ErrInvalid, "id is required")
	}
	order, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Order{}, fmt.Errorf("get order %s: %w", id, err)
	}
	return order, nil
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
