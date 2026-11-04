// Package dto defines the public JSON contract (requests and responses) and the
// mappings from model types. Kept separate from model so the API can evolve
// without touching business code, and vice versa.
package dto

import (
	"encoding/base64"
	"time"

	"github.com/yourorg/wallet-service/services/auth/internal/model"
)

// ---- shared ----

type ErrorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Service   string `json:"service"`
	RequestID string `json:"request_id,omitempty"`
}

// ---- auth ----

type CredentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type UserResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

func ToUserResponse(u model.User) UserResponse {
	return UserResponse{ID: u.ID, Email: u.Email, Role: u.Role, CreatedAt: u.CreatedAt}
}

type TokenResponse struct {
	TokenType        string    `json:"token_type"`
	AccessToken      string    `json:"access_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshToken     string    `json:"refresh_token"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

func ToTokenResponse(p model.TokenPair) TokenResponse {
	return TokenResponse{"Bearer", p.AccessToken, p.AccessExpiresAt, p.RefreshToken, p.RefreshExpiresAt}
}

// ---- JWKS (RFC 7517) ----

type JWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	X   string `json:"x"`
}

type JWKSResponse struct {
	Keys []JWK `json:"keys"`
}

// ToJWKS renders Ed25519 public keys as an OKP key set.
func ToJWKS(keys []model.PublicKey) JWKSResponse {
	out := JWKSResponse{Keys: make([]JWK, 0, len(keys))}
	for _, k := range keys {
		out.Keys = append(out.Keys, JWK{
			Kty: "OKP", Crv: "Ed25519", Alg: k.Algorithm, Use: "sig", Kid: k.KeyID,
			X: base64.RawURLEncoding.EncodeToString(k.Key),
		})
	}
	return out
}
