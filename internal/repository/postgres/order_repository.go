// Package postgres is an OUTBOUND ADAPTER: it implements service.OrderRepository
// using PostgreSQL. Only this package knows SQL.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/order-service/internal/domain"
)

const uniqueViolation = "23505"

type OrderRepository struct {
	pool *pgxpool.Pool
}

func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{pool: pool}
}

func (r *OrderRepository) Create(ctx context.Context, o domain.Order) error {
	const q = `
		INSERT INTO orders (id, customer_id, product_id, quantity, amount_cents, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := r.pool.Exec(ctx, q,
		o.ID, o.CustomerID, o.ProductID, o.Quantity, o.AmountCents, string(o.Status), o.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return domain.NewError(domain.ErrConflict, "order already exists")
		}
		return fmt.Errorf("insert order: %w", err)
	}
	return nil
}

func (r *OrderRepository) GetByID(ctx context.Context, id string) (domain.Order, error) {
	const q = `
		SELECT id, customer_id, product_id, quantity, amount_cents, status, created_at
		FROM orders
		WHERE id = $1`

	var (
		o      domain.Order
		status string
	)
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&o.ID, &o.CustomerID, &o.ProductID, &o.Quantity, &o.AmountCents, &status, &o.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Order{}, domain.NewError(domain.ErrNotFound, "order not found")
		}
		return domain.Order{}, fmt.Errorf("select order: %w", err)
	}
	o.Status = domain.OrderStatus(status)
	return o, nil
}
