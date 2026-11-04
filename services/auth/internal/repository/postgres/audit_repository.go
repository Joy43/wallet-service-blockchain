package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/wallet-service/services/auth/internal/model"
)

type AuditRepository struct{ pool *pgxpool.Pool }

func NewAuditRepository(pool *pgxpool.Pool) *AuditRepository { return &AuditRepository{pool: pool} }

// Record is idempotent on event_id so JetStream redeliveries are harmless.
func (r *AuditRepository) Record(ctx context.Context, e model.AuthEvent) error {
	const q = `
		INSERT INTO auth_audit_log (event_id, type, user_id, email, ip, occurred_at)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (event_id) DO NOTHING`
	if _, err := r.pool.Exec(ctx, q, e.ID, e.Type, e.UserID, e.Email, e.IP, e.OccurredAt); err != nil {
		return fmt.Errorf("insert audit row: %w", err)
	}
	return nil
}

func (r *AuditRepository) DeleteOlderThan(ctx context.Context, t time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM auth_audit_log WHERE occurred_at < $1`, t)
	if err != nil {
		return 0, fmt.Errorf("purge audit rows: %w", err)
	}
	return tag.RowsAffected(), nil
}
