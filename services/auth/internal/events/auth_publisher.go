package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/yourorg/wallet-service/services/auth/internal/model"
)

const (
	AuthStream        = "AUTH"
	AuthSubjectPrefix = "auth."
	AuthSubjectFilter = "auth.>"
)

type AuthPublisher struct{ js jetstream.JetStream }

func NewAuthPublisher(js jetstream.JetStream) *AuthPublisher { return &AuthPublisher{js: js} }

// PublishAuthEvent publishes to auth.<event type>, e.g. auth.user.logged_in.
func (p *AuthPublisher) PublishAuthEvent(ctx context.Context, e model.AuthEvent) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal auth event: %w", err)
	}
	return p.PublishRaw(ctx, AuthSubjectPrefix+e.Type, data, e.ID)
}

// PublishRaw publishes an already-encoded message. msgID enables JetStream de-duplication,
// so re-publishing the same outbox row is harmless.
func (p *AuthPublisher) PublishRaw(ctx context.Context, subject string, data []byte, msgID string) error {
	if _, err := p.js.Publish(ctx, subject, data, jetstream.WithMsgID(msgID)); err != nil {
		return fmt.Errorf("publish %s: %w", subject, err)
	}
	return nil
}
