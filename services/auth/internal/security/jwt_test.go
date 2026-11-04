package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func newIssuer(t *testing.T, kid, iss string) *JWTIssuer {
	t.Helper()
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	j, err := NewJWTIssuer(key, kid, iss, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func TestJWT_RoundTrip(t *testing.T) {
	j := newIssuer(t, "k1", "auth-service")
	tok, _, err := j.Issue("u1", "admin", "s1")
	if err != nil {
		t.Fatal(err)
	}
	c, err := j.Parse(tok)
	if err != nil {
		t.Fatal(err)
	}
	if c.UserID != "u1" || c.Role != "admin" || c.SessionID != "s1" {
		t.Errorf("claims = %+v", c)
	}
}

// Other services verify with only the PUBLIC key: this is the whole point of Ed25519.
func TestJWT_VerifiableWithPublicKeyOnly(t *testing.T) {
	j := newIssuer(t, "k1", "auth-service")
	tok, _, _ := j.Issue("u1", "user", "s1")

	pk := j.PublicKeys()
	if len(pk) != 1 || pk[0].KeyID != "k1" || pk[0].Algorithm != "EdDSA" {
		t.Fatalf("public keys = %+v", pk)
	}
	var c accessClaims
	_, err := jwt.ParseWithClaims(tok, &c, func(*jwt.Token) (any, error) { return ed25519.PublicKey(pk[0].Key), nil },
		jwt.WithValidMethods([]string{"EdDSA"}))
	if err != nil {
		t.Fatalf("verify with public key: %v", err)
	}
}

func TestJWT_Rejects(t *testing.T) {
	j := newIssuer(t, "k1", "auth-service")
	good, _, _ := j.Issue("u1", "user", "s1")

	foreignKey, _, _ := newIssuer(t, "k1", "auth-service").Issue("u1", "user", "s1")
	wrongKid, _, _ := newIssuer(t, "other", "auth-service").Issue("u1", "user", "s1")
	wrongIssuer, _, _ := newIssuer(t, "k1", "someone-else").Issue("u1", "user", "s1")

	expired := newIssuer(t, "k1", "auth-service")
	expired.priv, expired.pub = j.priv, j.pub
	expired.now = func() time.Time { return time.Now().Add(-time.Hour) }
	old, _, _ := expired.Issue("u1", "user", "s1")

	// alg=none and HS256 signed with the public key (classic confusion attack) must fail.
	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, accessClaims{
		SessionID: "s1", RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "auth-service", Subject: "u1", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))},
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims{
		SessionID: "s1", RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "auth-service", Subject: "u1", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))},
	})
	hs.Header["kid"] = "k1"
	hsTok, _ := hs.SignedString([]byte(j.pub))

	for name, tok := range map[string]string{
		"wrong key": foreignKey, "wrong kid": wrongKid, "wrong issuer": wrongIssuer, "expired": old,
		"alg none": none, "hs256 confusion": hsTok,
		"tampered": good[:len(good)-2] + "xx", "empty": "",
	} {
		if _, err := j.Parse(tok); err == nil {
			t.Errorf("%s: token accepted", name)
		}
	}
}

func TestJWT_BadKeyRejected(t *testing.T) {
	for name, key := range map[string]string{
		"empty": "", "not base64": "%%%", "not pem": "aGVsbG8=",
		"short": strings.Repeat("A", 8),
	} {
		if _, err := NewJWTIssuer(key, "k1", "x", time.Minute); err == nil {
			t.Errorf("%s: bad key accepted", name)
		}
	}
	if _, err := NewJWTIssuer("x", "", "x", time.Minute); err == nil {
		t.Error("empty key id accepted")
	}
	_ = rand.Reader
}
