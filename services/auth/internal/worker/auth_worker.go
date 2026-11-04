// Package worker contains background processing for the auth service:
// a durable JetStream consumer that writes the audit trail, and a janitor
// that enforces audit-log retention.
package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/yourorg/wallet-service/services/auth/internal/model"
)

type AuditStore interface {
	Record(ctx context.Context, e model.AuthEvent) error
	DeleteOlderThan(ctx context.Context, t time.Time) (int64, error)
}

type AuthWorker struct {
	consumer  jetstream.Consumer
	store     AuditStore
	retention time.Duration
	log       *slog.Logger
}

func NewAuthWorker(c jetstream.Consumer, store AuditStore, retention time.Duration, log *slog.Logger) *AuthWorker {
	return &AuthWorker{consumer: c, store: store, retention: retention, log: log}
}

// Run blocks until ctx is cancelled.
func (w *AuthWorker) Run(ctx context.Context) error {
	cc, err := w.consumer.Consume(w.handle)
	if err != nil {
		return err
	}
	defer cc.Stop()

	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			n, err := w.store.DeleteOlderThan(ctx, time.Now().Add(-w.retention))
			if err != nil {
				w.log.ErrorContext(ctx, "audit retention purge failed", slog.Any("error", err))
			} else if n > 0 {
				w.log.InfoContext(ctx, "purged old audit rows", slog.Int64("rows", n))
			}
		}
	}
}

func (w *AuthWorker) handle(msg jetstream.Msg) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var e model.AuthEvent
	if err := json.Unmarshal(msg.Data(), &e); err != nil || e.ID == "" {
		w.log.Error("dropping malformed auth event", slog.String("subject", msg.Subject()), slog.Any("error", err))
		_ = msg.Term() // poison message: redelivery can never succeed
		return
	}
	if err := w.store.Record(ctx, e); err != nil {
		w.log.Error("record audit event failed, will retry", slog.String("event_id", e.ID), slog.Any("error", err))
		_ = msg.NakWithDelay(5 * time.Second)
		return
	}
	if e.Type == model.AuthEventLoginFailed || e.Type == model.AuthEventTokenReuse {
		w.log.Warn("security event", slog.String("type", e.Type), slog.String("email", e.Email), slog.String("ip", e.IP))
	}
	_ = msg.Ack()
}
