package controller

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/yourorg/wallet-service/gen/auth/v1"
	"github.com/yourorg/wallet-service/services/auth/internal/model"
)

type stubAuth struct {
	AuthService // unimplemented methods panic if called: tests only use PublicKeys
}

func (stubAuth) PublicKeys() []model.PublicKey {
	return []model.PublicKey{{KeyID: "k1", Algorithm: "EdDSA", Key: []byte{1, 2, 3}}}
}

func TestJWKS(t *testing.T) {
	c := NewAuthController(stubAuth{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := httptest.NewRecorder()
	c.JWKS(rec, httptest.NewRequest("GET", "/.well-known/jwks.json", nil))

	if rec.Code != 200 || rec.Header().Get("Cache-Control") == "" {
		t.Fatalf("status=%d cache=%q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	var body struct {
		Keys []map[string]string `json:"keys"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	k := body.Keys[0]
	if k["kty"] != "OKP" || k["crv"] != "Ed25519" || k["alg"] != "EdDSA" || k["use"] != "sig" || k["kid"] != "k1" {
		t.Errorf("jwk = %v", k)
	}
	if k["x"] != base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3}) {
		t.Errorf("x = %q", k["x"])
	}
	for _, private := range []string{"d", "private"} {
		if _, ok := k[private]; ok {
			t.Errorf("JWKS leaked private field %q", private)
		}
	}
}

type stubValidator struct {
	claims model.Claims
	err    error
}

func (s stubValidator) ValidateToken(context.Context, string) (model.Claims, error) {
	return s.claims, s.err
}

func TestGRPCValidateToken(t *testing.T) {
	ok := NewGRPCAuthController(stubValidator{claims: model.Claims{UserID: "u1", Role: "user", SessionID: "s1", ExpiresAt: time.Unix(1000, 0)}})
	resp, err := ok.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{AccessToken: "t"})
	if err != nil || resp.GetUserId() != "u1" || resp.GetSessionId() != "s1" || resp.GetExpiresAtUnix() != 1000 {
		t.Fatalf("resp=%v err=%v", resp, err)
	}

	for name, tt := range map[string]struct {
		err  error
		code codes.Code
	}{
		"unauthenticated": {model.NewError(model.ErrUnauthenticated, "x"), codes.Unauthenticated},
		"unavailable":     {model.NewError(model.ErrUnavailable, "x"), codes.Unavailable},
		"unknown error":   {errors.New("db password leaked"), codes.Internal},
	} {
		c := NewGRPCAuthController(stubValidator{err: tt.err})
		_, err := c.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{})
		if status.Code(err) != tt.code {
			t.Errorf("%s: code = %v, want %v", name, status.Code(err), tt.code)
		}
		if status.Code(err) == codes.Internal && status.Convert(err).Message() != "internal error" {
			t.Errorf("%s: internal detail leaked: %q", name, status.Convert(err).Message())
		}
	}
}
