// Package events is an OUTBOUND ADAPTER: it implements service.EventPublisher
// using NATS JetStream. Swap it for Kafka without touching business logic.
package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/yourorg/order-service/internal/domain"
)

const SubjectOrderCreated = "orders.created"

type Publisher struct {
	js jetstream.JetStream
}

func NewPublisher(js jetstream.JetStream) *Publisher {
	return &Publisher{js: js}
}

func (p *Publisher) PublishOrderCreated(ctx context.Context, e domain.OrderCreated) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal OrderCreated: %w", err)
	}
	// MsgID enables JetStream de-duplication if the publish is retried.
	if _, err := p.js.Publish(ctx, SubjectOrderCreated, data, jetstream.WithMsgID(e.OrderID)); err != nil {
		return fmt.Errorf("publish %s: %w", SubjectOrderCreated, err)
	}
	return nil
}
