package worker

import (
	"context"
	"log/slog"
	"time"
)

const outboxBatch = 100

// OutboxStore is implemented by the Postgres outbox repository.
type OutboxStore interface {
	PublishPending(ctx context.Context, limit int, publish func(ctx context.Context, subject string, payload []byte, msgID string) error) (int, error)
	DeletePublishedBefore(ctx context.Context, t time.Time) (int64, error)
}

// OutboxRelay moves events from the outbox table to the broker. Because the event row
// is committed together with the business change, an event is never lost if the broker is
// down; it is simply published later (at-least-once; consumers de-duplicate by event id).
type OutboxRelay struct {
	store    OutboxStore
	publish  func(ctx context.Context, subject string, payload []byte, msgID string) error
	interval time.Duration
	log      *slog.Logger
}

func NewOutboxRelay(store OutboxStore, publish func(ctx context.Context, subject string, payload []byte, msgID string) error, interval time.Duration, log *slog.Logger) *OutboxRelay {
	return &OutboxRelay{store: store, publish: publish, interval: interval, log: log}
}

// Run blocks until ctx is cancelled.
func (r *OutboxRelay) Run(ctx context.Context) error {
	tick := time.NewTicker(r.interval)
	defer tick.Stop()
	purge := time.NewTicker(time.Hour)
	defer purge.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			r.drain(ctx)
		case <-purge.C:
			if n, err := r.store.DeletePublishedBefore(ctx, time.Now().Add(-7*24*time.Hour)); err != nil {
				r.log.ErrorContext(ctx, "outbox purge failed", slog.Any("error", err))
			} else if n > 0 {
				r.log.InfoContext(ctx, "purged published outbox rows", slog.Int64("rows", n))
			}
		}
	}
}

// drain publishes batches until the table is empty or an error occurs.
func (r *OutboxRelay) drain(ctx context.Context) {
	for {
		n, err := r.store.PublishPending(ctx, outboxBatch, r.publish)
		if err != nil {
			r.log.WarnContext(ctx, "outbox publish failed, will retry", slog.Any("error", err))
			return
		}
		if n < outboxBatch {
			return
		}
	}
}
