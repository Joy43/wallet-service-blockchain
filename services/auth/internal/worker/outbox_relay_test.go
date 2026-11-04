package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

type fakeOutbox struct {
	pending []string // subjects
	failAt  int      // publish call number (1-based) that fails; 0 = never
}

func (f *fakeOutbox) PublishPending(ctx context.Context, limit int, publish func(context.Context, string, []byte, string) error) (int, error) {
	n := 0
	for len(f.pending) > 0 && n < limit {
		if f.failAt == n+1 {
			return n, errors.New("broker down")
		}
		if err := publish(ctx, f.pending[0], nil, "id"); err != nil {
			return n, err
		}
		f.pending = f.pending[1:]
		n++
	}
	return n, nil
}
func (f *fakeOutbox) DeletePublishedBefore(context.Context, time.Time) (int64, error) { return 0, nil }

func newRelay(f *fakeOutbox, sent *[]string) *OutboxRelay {
	return NewOutboxRelay(f, func(_ context.Context, subject string, _ []byte, _ string) error {
		*sent = append(*sent, subject)
		return nil
	}, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestOutboxRelay_DrainsEverything(t *testing.T) {
	f := &fakeOutbox{}
	for i := 0; i < 250; i++ { // more than two batches
		f.pending = append(f.pending, "auth.user.registered")
	}
	var sent []string
	newRelay(f, &sent).drain(context.Background())
	if len(sent) != 250 || len(f.pending) != 0 {
		t.Errorf("sent=%d pending=%d, want 250/0", len(sent), len(f.pending))
	}
}

func TestOutboxRelay_BrokerDownKeepsRows(t *testing.T) {
	f := &fakeOutbox{pending: []string{"a", "b", "c"}, failAt: 1}
	var sent []string
	newRelay(f, &sent).drain(context.Background())
	if len(f.pending) != 3 {
		t.Errorf("rows were dropped while broker was down: pending=%d", len(f.pending))
	}
}
