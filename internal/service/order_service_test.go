package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/yourorg/order-service/internal/domain"
)

// Fakes: possible only because the service depends on interfaces.
type fakeRepo struct{ orders map[string]domain.Order }

func (f *fakeRepo) Create(_ context.Context, o domain.Order) error {
	f.orders[o.ID] = o
	return nil
}

func (f *fakeRepo) GetByID(_ context.Context, id string) (domain.Order, error) {
	o, ok := f.orders[id]
	if !ok {
		return domain.Order{}, domain.NewError(domain.ErrNotFound, "order not found")
	}
	return o, nil
}

type fakePublisher struct {
	published []domain.OrderCreated
	err       error
}

func (f *fakePublisher) PublishOrderCreated(_ context.Context, e domain.OrderCreated) error {
	if f.err != nil {
		return f.err
	}
	f.published = append(f.published, e)
	return nil
}

func newTestService(pub *fakePublisher) (*OrderService, *fakeRepo) {
	repo := &fakeRepo{orders: map[string]domain.Order{}}
	svc := NewOrderService(repo, pub, slog.New(slog.NewTextHandler(io.Discard, nil)))
	svc.newID = func() string { return "order-1" }
	svc.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	return svc, repo
}

func TestCreateOrder_SavesAndPublishes(t *testing.T) {
	pub := &fakePublisher{}
	svc, repo := newTestService(pub)

	order, err := svc.CreateOrder(context.Background(), domain.CreateOrderInput{
		CustomerID: "c-1", ProductID: "p-1", Quantity: 2, AmountCents: 1999,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if order.Status != domain.StatusPending {
		t.Errorf("status = %s, want PENDING", order.Status)
	}
	if _, ok := repo.orders["order-1"]; !ok {
		t.Error("order was not saved")
	}
	if len(pub.published) != 1 || pub.published[0].OrderID != "order-1" {
		t.Errorf("expected one OrderCreated event, got %+v", pub.published)
	}
}

func TestCreateOrder_InvalidInput(t *testing.T) {
	svc, _ := newTestService(&fakePublisher{})
	_, err := svc.CreateOrder(context.Background(), domain.CreateOrderInput{})
	if domain.CodeOf(err) != domain.ErrInvalid {
		t.Fatalf("code = %s, want INVALID_ARGUMENT", domain.CodeOf(err))
	}
}

func TestCreateOrder_PublishFailureStillReturnsOrder(t *testing.T) {
	svc, _ := newTestService(&fakePublisher{err: errors.New("nats down")})
	_, err := svc.CreateOrder(context.Background(), domain.CreateOrderInput{
		CustomerID: "c-1", ProductID: "p-1", Quantity: 1, AmountCents: 500,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestGetOrder_NotFound(t *testing.T) {
	svc, _ := newTestService(&fakePublisher{})
	_, err := svc.GetOrder(context.Background(), "missing")
	if domain.CodeOf(err) != domain.ErrNotFound {
		t.Fatalf("code = %s, want NOT_FOUND", domain.CodeOf(err))
	}
}
