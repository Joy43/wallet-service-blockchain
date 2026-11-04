package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type OutboxRepository struct{ pool *pgxpool.Pool }

func NewOutboxRepository(pool *pgxpool.Pool) *OutboxRepository { return &OutboxRepository{pool: pool} }

type outboxRow struct{ id, subject, payload string }

// PublishPending claims up to limit unpublished rows (FOR UPDATE SKIP LOCKED, so several
// relay instances never publish the same row), publishes them in order, and marks the
// successful ones. If publishing fails midway, rows already sent stay marked and the error is returned.
func (r *OutboxRepository) PublishPending(ctx context.Context, limit int, publish func(ctx context.Context, subject string, payload []byte, msgID string) error) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id, subject, payload FROM auth_outbox
		WHERE published_at IS NULL
		ORDER BY created_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return 0, fmt.Errorf("select outbox: %w", err)
	}
	var batch []outboxRow
	for rows.Next() {
		var o outboxRow
		if err := rows.Scan(&o.id, &o.subject, &o.payload); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan outbox: %w", err)
		}
		batch = append(batch, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("read outbox: %w", err)
	}

	var sent []string
	var pubErr error
	for _, o := range batch {
		if pubErr = publish(ctx, o.subject, []byte(o.payload), o.id); pubErr != nil {
			break
		}
		sent = append(sent, o.id)
	}
	if len(sent) > 0 {
		if _, err := tx.Exec(ctx, `UPDATE auth_outbox SET published_at = now() WHERE id = ANY($1)`, sent); err != nil {
			return 0, fmt.Errorf("mark published: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return len(sent), pubErr
}

func (r *OutboxRepository) DeletePublishedBefore(ctx context.Context, t time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM auth_outbox WHERE published_at IS NOT NULL AND published_at < $1`, t)
	if err != nil {
		return 0, fmt.Errorf("purge outbox: %w", err)
	}
	return tag.RowsAffected(), nil
}
