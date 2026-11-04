package service

import (
	"context"
	"time"

	"github.com/yourorg/wallet-service/services/auth/internal/model"
)

type UserRepository interface {
	// CreateWithEvent stores the user and the event in ONE transaction (outbox pattern),
	// so the event is published if and only if the user exists. ErrConflict if the email exists.
	CreateWithEvent(ctx context.Context, u model.User, e model.AuthEvent) error
	GetByEmail(ctx context.Context, email string) (model.User, error)
	GetByID(ctx context.Context, id string) (model.User, error)
}

// UserCache is a cache-aside layer in front of UserRepository.GetByID.
type UserCache interface {
	Get(ctx context.Context, id string) (model.User, bool, error)
	Set(ctx context.Context, u model.User, ttl time.Duration) error
}

// SessionStore holds refresh-token sessions (Redis).
type SessionStore interface {
	Create(ctx context.Context, s model.Session, ttl time.Duration) error
	// Rotate atomically swaps the refresh secret. It returns ErrUnauthenticated
	// if the session is gone and ErrTokenReuse if oldHash is not the current secret.
	Rotate(ctx context.Context, sessionID, oldHash, newHash string, ttl time.Duration) (model.Session, error)
	Active(ctx context.Context, sessionID string) (bool, error)
	Revoke(ctx context.Context, sessionID string) error
}

type RateLimiter interface {
	// Allow records one attempt for key and reports whether it is within limit per window.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

type TokenIssuer interface {
	Issue(userID, role, sessionID string) (token string, expiresAt time.Time, err error)
	Parse(token string) (model.Claims, error)
	PublicKeys() []model.PublicKey
}

type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}

type AuthEventPublisher interface {
	PublishAuthEvent(ctx context.Context, e model.AuthEvent) error
}
