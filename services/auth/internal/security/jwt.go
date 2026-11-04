package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/yourorg/wallet-service/services/auth/internal/model"
)

// JWTIssuer signs access tokens with Ed25519 (EdDSA). Only auth-service holds the
// private key; every other service verifies tokens with the public key from the
// JWKS endpoint, so no shared secret ever leaves this service.
type JWTIssuer struct {
	priv   ed25519.PrivateKey
	pub    ed25519.PublicKey
	kid    string
	issuer string
	ttl    time.Duration
	now    func() time.Time
}

// NewJWTIssuer takes the private key as base64(PEM(PKCS#8)): one line, safe in an env var.
func NewJWTIssuer(privateKeyB64, keyID, issuer string, ttl time.Duration) (*JWTIssuer, error) {
	if keyID == "" {
		return nil, errors.New("JWT key id is required")
	}
	priv, err := parsePrivateKey(privateKeyB64)
	if err != nil {
		return nil, err
	}
	return &JWTIssuer{
		priv: priv, pub: priv.Public().(ed25519.PublicKey),
		kid: keyID, issuer: issuer, ttl: ttl, now: time.Now,
	}, nil
}

// GenerateKey returns a new signing key in the format NewJWTIssuer expects.
func GenerateKey() (string, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", fmt.Errorf("marshal key: %w", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return base64.StdEncoding.EncodeToString(pemBytes), nil
}

func parsePrivateKey(b64 string) (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, errors.New("JWT private key is not valid base64")
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("JWT private key is not valid PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse JWT private key: %w", err)
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("JWT private key must be Ed25519")
	}
	return priv, nil
}

type accessClaims struct {
	Role      string `json:"role"`
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

func (j *JWTIssuer) Issue(userID, role, sessionID string) (string, time.Time, error) {
	now := j.now()
	exp := now.Add(j.ttl)
	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, accessClaims{
		Role: role, SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: j.issuer, Subject: userID,
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(exp),
		},
	})
	tok.Header["kid"] = j.kid
	s, err := tok.SignedString(j.priv)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return s, exp.UTC(), nil
}

func (j *JWTIssuer) Parse(token string) (model.Claims, error) {
	var c accessClaims
	_, err := jwt.ParseWithClaims(token, &c,
		func(t *jwt.Token) (any, error) {
			if kid, _ := t.Header["kid"].(string); kid != j.kid {
				return nil, errors.New("unknown key id")
			}
			return j.pub, nil
		},
		// Pin the algorithm: never trust the token's own "alg" header.
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(j.issuer), jwt.WithExpirationRequired(), jwt.WithTimeFunc(j.now))
	if err != nil {
		return model.Claims{}, err
	}
	if c.Subject == "" || c.SessionID == "" {
		return model.Claims{}, errors.New("token missing subject or session")
	}
	return model.Claims{UserID: c.Subject, Role: c.Role, SessionID: c.SessionID, ExpiresAt: c.ExpiresAt.Time}, nil
}

// PublicKeys lists the keys other services may use to verify tokens (served as JWKS).
func (j *JWTIssuer) PublicKeys() []model.PublicKey {
	return []model.PublicKey{{KeyID: j.kid, Algorithm: jwt.SigningMethodEdDSA.Alg(), Key: j.pub}}
}
