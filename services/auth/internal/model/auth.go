package model

import (
	"net/mail"
	"strings"
	"time"
)

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

type User struct {
	ID           string
	Email        string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
}

// Session is the server-side record behind a refresh token. Revoking it
// (logout, refresh-token reuse) instantly invalidates every access token
// issued for it.
type Session struct {
	ID         string
	UserID     string
	Role       string
	SecretHash string // sha256 of the current refresh secret
}

// Claims is what a validated access token proves.
type Claims struct {
	UserID    string
	Role      string
	SessionID string
	ExpiresAt time.Time
}

// PublicKey is a token-verification key published via JWKS.
type PublicKey struct {
	KeyID     string
	Algorithm string
	Key       []byte
}

type TokenPair struct {
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshExpiresAt time.Time
}

type RegisterInput struct {
	Email    string
	Password string
}

func (in *RegisterInput) Validate() error {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if _, err := mail.ParseAddress(in.Email); err != nil {
		return NewError(ErrInvalid, "email is invalid")
	}
	// bcrypt only uses the first 72 bytes; reject longer input instead of truncating silently.
	if len(in.Password) < 8 || len(in.Password) > 72 {
		return NewError(ErrInvalid, "password must be 8-72 characters")
	}
	return nil
}

type LoginInput struct {
	Email    string
	Password string
	IP       string
}

// AuthEvent is published for every security-relevant action. The worker
// persists them as an audit trail; other services may subscribe too.
type AuthEvent struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"` // one of the AuthEvent* constants
	UserID     string    `json:"user_id,omitempty"`
	Email      string    `json:"email,omitempty"`
	IP         string    `json:"ip,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

const (
	AuthEventUserRegistered = "user.registered"
	AuthEventUserLoggedIn   = "user.logged_in"
	AuthEventLoginFailed    = "login.failed"
	AuthEventUserLoggedOut  = "user.logged_out"
	AuthEventTokenReuse     = "token.reuse_detected"
)
