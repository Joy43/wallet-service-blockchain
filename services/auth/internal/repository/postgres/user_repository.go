// Package postgres is an OUTBOUND ADAPTER: it implements the service ports with
// PostgreSQL. Only this package knows SQL.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/wallet-service/services/auth/internal/model"
)

// PostgreSQL error code for unique_violation.
const uniqueViolation = "23505"

type UserRepository struct{ pool *pgxpool.Pool }

func NewUserRepository(pool *pgxpool.Pool) *UserRepository { return &UserRepository{pool: pool} }

// CreateWithEvent inserts the user and its outbox event atomically.
func (r *UserRepository) CreateWithEvent(ctx context.Context, u model.User, e model.AuthEvent) error {
	payload, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	const insertUser = `INSERT INTO users (id, email, password_hash, role, created_at) VALUES ($1,$2,$3,$4,$5)`
	if _, err := tx.Exec(ctx, insertUser, u.ID, u.Email, u.PasswordHash, u.Role, u.CreatedAt); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return model.NewError(model.ErrConflict, "email already registered")
		}
		return fmt.Errorf("insert user: %w", err)
	}
	const insertOutbox = `INSERT INTO auth_outbox (id, subject, payload) VALUES ($1,$2,$3)`
	if _, err := tx.Exec(ctx, insertOutbox, e.ID, "auth."+e.Type, string(payload)); err != nil {
		return fmt.Errorf("insert outbox: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (model.User, error) {
	return r.get(ctx, `WHERE email = $1`, email)
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (model.User, error) {
	return r.get(ctx, `WHERE id = $1`, id)
}

func (r *UserRepository) get(ctx context.Context, where string, arg string) (model.User, error) {
	var u model.User
	err := r.pool.QueryRow(ctx,
		`SELECT id, email, password_hash, role, created_at FROM users `+where, arg).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.User{}, model.NewError(model.ErrNotFound, "user not found")
	}
	if err != nil {
		return model.User{}, fmt.Errorf("select user: %w", err)
	}
	return u, nil
}
