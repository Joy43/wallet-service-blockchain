package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/wallet-service/services/auth/internal/model"
)

// ---- fakes ----

type fakeUsers struct {
	byEmail map[string]model.User
	outbox  []model.AuthEvent // events stored atomically with users
}

func (f *fakeUsers) CreateWithEvent(_ context.Context, u model.User, e model.AuthEvent) error {
	if _, ok := f.byEmail[u.Email]; ok {
		return model.NewError(model.ErrConflict, "email already registered")
	}
	f.byEmail[u.Email] = u
	f.outbox = append(f.outbox, e)
	return nil
}
func (f *fakeUsers) GetByEmail(_ context.Context, e string) (model.User, error) {
	if u, ok := f.byEmail[e]; ok {
		return u, nil
	}
	return model.User{}, model.NewError(model.ErrNotFound, "user not found")
}
func (f *fakeUsers) GetByID(_ context.Context, id string) (model.User, error) {
	for _, u := range f.byEmail {
		if u.ID == id {
			return u, nil
		}
	}
	return model.User{}, model.NewError(model.ErrNotFound, "user not found")
}

type fakeCache struct{ m map[string]model.User }

func (f *fakeCache) Get(_ context.Context, id string) (model.User, bool, error) {
	u, ok := f.m[id]
	return u, ok, nil
}
func (f *fakeCache) Set(_ context.Context, u model.User, _ time.Duration) error {
	f.m[u.ID] = u
	return nil
}

type fakeSessions struct{ m map[string]model.Session }

func (f *fakeSessions) Create(_ context.Context, s model.Session, _ time.Duration) error {
	f.m[s.ID] = s
	return nil
}
func (f *fakeSessions) Rotate(_ context.Context, id, oldHash, newHash string, _ time.Duration) (model.Session, error) {
	s, ok := f.m[id]
	if !ok {
		return model.Session{}, model.NewError(model.ErrUnauthenticated, "session not found")
	}
	if s.SecretHash != oldHash {
		return model.Session{}, ErrTokenReuse
	}
	s.SecretHash = newHash
	f.m[id] = s
	return s, nil
}
func (f *fakeSessions) Active(_ context.Context, id string) (bool, error) {
	_, ok := f.m[id]
	return ok, nil
}
func (f *fakeSessions) Revoke(_ context.Context, id string) error {
	delete(f.m, id)
	return nil
}

type fakeLimiter struct{ count, limit int }

func (f *fakeLimiter) Allow(context.Context, string, int, time.Duration) (bool, error) {
	f.count++
	return f.count <= f.limit, nil
}

// fakeTokens: token = "tok|user|role|sid"; avoids real JWT in service tests.
type fakeTokens struct{}

func (fakeTokens) Issue(u, r, sid string) (string, time.Time, error) {
	return "tok|" + u + "|" + r + "|" + sid, time.Now().Add(time.Minute), nil
}
func (fakeTokens) PublicKeys() []model.PublicKey {
	return []model.PublicKey{{KeyID: "k", Algorithm: "EdDSA", Key: []byte("pub")}}
}
func (fakeTokens) Parse(t string) (model.Claims, error) {
	p := strings.Split(t, "|")
	if len(p) != 4 || p[0] != "tok" {
		return model.Claims{}, errors.New("bad token")
	}
	return model.Claims{UserID: p[1], Role: p[2], SessionID: p[3]}, nil
}

type fakeHasher struct{}

func (fakeHasher) Hash(p string) (string, error) { return "h:" + p, nil }
func (fakeHasher) Compare(h, p string) error {
	if h != "h:"+p {
		return errors.New("mismatch")
	}
	return nil
}

type fakeEvents struct{ types []string }

func (f *fakeEvents) PublishAuthEvent(_ context.Context, e model.AuthEvent) error {
	f.types = append(f.types, e.Type)
	return nil
}

type authRig struct {
	svc      *AuthService
	sessions *fakeSessions
	events   *fakeEvents
	limiter  *fakeLimiter
}

func newAuthRig(t *testing.T) authRig {
	t.Helper()
	r := authRig{
		sessions: &fakeSessions{m: map[string]model.Session{}},
		events:   &fakeEvents{},
		limiter:  &fakeLimiter{limit: 100},
	}
	svc, err := NewAuthService(&fakeUsers{byEmail: map[string]model.User{}}, &fakeCache{m: map[string]model.User{}},
		r.sessions, r.limiter, fakeTokens{}, fakeHasher{}, r.events,
		AuthConfig{RefreshTTL: time.Hour, UserCacheTTL: time.Minute, LoginLimit: 5, LoginWindow: time.Minute},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	r.svc = svc
	return r
}

func (r authRig) registerAndLogin(t *testing.T) model.TokenPair {
	t.Helper()
	ctx := context.Background()
	if _, err := r.svc.Register(ctx, model.RegisterInput{Email: "A@Example.com", Password: "password123"}); err != nil {
		t.Fatalf("register: %v", err)
	}
	pair, err := r.svc.Login(ctx, model.LoginInput{Email: "a@example.com", Password: "password123", IP: "1.2.3.4"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return pair
}

// ---- tests ----

func TestRegister_ValidationAndDuplicate(t *testing.T) {
	r := newAuthRig(t)
	ctx := context.Background()

	for _, in := range []model.RegisterInput{
		{Email: "not-an-email", Password: "password123"},
		{Email: "a@example.com", Password: "short"},
		{Email: "a@example.com", Password: strings.Repeat("x", 73)},
	} {
		if got := model.CodeOf(func() error { _, err := r.svc.Register(ctx, in); return err }()); got != model.ErrInvalid {
			t.Errorf("Register(%+v) code = %v, want INVALID_ARGUMENT", in, got)
		}
	}

	in := model.RegisterInput{Email: "a@example.com", Password: "password123"}
	if _, err := r.svc.Register(ctx, in); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.Register(ctx, in); model.CodeOf(err) != model.ErrConflict {
		t.Errorf("duplicate register code = %v, want CONFLICT", model.CodeOf(err))
	}
}

func TestLogin_SameErrorForUnknownEmailAndWrongPassword(t *testing.T) {
	r := newAuthRig(t)
	ctx := context.Background()
	r.registerAndLogin(t)

	_, errWrongPw := r.svc.Login(ctx, model.LoginInput{Email: "a@example.com", Password: "wrong-password"})
	_, errNoUser := r.svc.Login(ctx, model.LoginInput{Email: "nobody@example.com", Password: "password123"})

	for _, err := range []error{errWrongPw, errNoUser} {
		if model.CodeOf(err) != model.ErrUnauthenticated {
			t.Fatalf("code = %v, want UNAUTHENTICATED", model.CodeOf(err))
		}
	}
	if errWrongPw.Error() != errNoUser.Error() {
		t.Errorf("messages differ (%q vs %q): account enumeration leak", errWrongPw, errNoUser)
	}
}

func TestLogin_RateLimited(t *testing.T) {
	r := newAuthRig(t)
	r.limiter.limit = 1
	r.registerAndLogin(t) // consumes the single allowed attempt

	_, err := r.svc.Login(context.Background(), model.LoginInput{Email: "a@example.com", Password: "password123"})
	if model.CodeOf(err) != model.ErrRateLimited {
		t.Errorf("code = %v, want RATE_LIMITED", model.CodeOf(err))
	}
}

func TestValidateToken_RevokedAfterLogout(t *testing.T) {
	r := newAuthRig(t)
	ctx := context.Background()
	pair := r.registerAndLogin(t)

	claims, err := r.svc.ValidateToken(ctx, pair.AccessToken)
	if err != nil {
		t.Fatalf("validate before logout: %v", err)
	}
	if err := r.svc.Logout(ctx, claims); err != nil {
		t.Fatal(err)
	}
	if _, err := r.svc.ValidateToken(ctx, pair.AccessToken); model.CodeOf(err) != model.ErrUnauthenticated {
		t.Errorf("validate after logout code = %v, want UNAUTHENTICATED", model.CodeOf(err))
	}
	if _, err := r.svc.ValidateToken(ctx, "garbage"); model.CodeOf(err) != model.ErrUnauthenticated {
		t.Errorf("garbage token code = %v, want UNAUTHENTICATED", model.CodeOf(err))
	}
}

func TestRefresh_RotatesAndDetectsReuse(t *testing.T) {
	r := newAuthRig(t)
	ctx := context.Background()
	first := r.registerAndLogin(t)

	second, err := r.svc.Refresh(ctx, first.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh token was not rotated")
	}

	// Replaying the old token is treated as theft: the whole session dies,
	// including the legitimately rotated token.
	if _, err := r.svc.Refresh(ctx, first.RefreshToken); model.CodeOf(err) != model.ErrUnauthenticated {
		t.Errorf("reuse code = %v, want UNAUTHENTICATED", model.CodeOf(err))
	}
	if _, err := r.svc.Refresh(ctx, second.RefreshToken); model.CodeOf(err) != model.ErrUnauthenticated {
		t.Errorf("session survived reuse detection: code = %v", model.CodeOf(err))
	}
	if _, err := r.svc.ValidateToken(ctx, second.AccessToken); model.CodeOf(err) != model.ErrUnauthenticated {
		t.Errorf("access token survived reuse detection: code = %v", model.CodeOf(err))
	}

	var sawReuse bool
	for _, typ := range r.events.types {
		sawReuse = sawReuse || typ == model.AuthEventTokenReuse
	}
	if !sawReuse {
		t.Errorf("no %s event published; got %v", model.AuthEventTokenReuse, r.events.types)
	}
}

func TestRefresh_MalformedToken(t *testing.T) {
	r := newAuthRig(t)
	for _, tok := range []string{"", "nodot", ".secret", "sid."} {
		if _, err := r.svc.Refresh(context.Background(), tok); model.CodeOf(err) != model.ErrUnauthenticated {
			t.Errorf("Refresh(%q) code = %v, want UNAUTHENTICATED", tok, model.CodeOf(err))
		}
	}
}

func TestMe_CacheAside(t *testing.T) {
	r := newAuthRig(t)
	ctx := context.Background()
	pair := r.registerAndLogin(t)
	claims, _ := r.svc.ValidateToken(ctx, pair.AccessToken)

	if _, err := r.svc.Me(ctx, claims.UserID); err != nil {
		t.Fatal(err)
	}
	// Remove from the DB: a second call must still succeed from cache.
	delete(r.svc.users.(*fakeUsers).byEmail, "a@example.com")
	if _, err := r.svc.Me(ctx, claims.UserID); err != nil {
		t.Errorf("expected cache hit, got %v", err)
	}
}

func TestRegister_WritesEventToOutboxNotDirectly(t *testing.T) {
	r := newAuthRig(t)
	users := r.svc.users.(*fakeUsers)
	if _, err := r.svc.Register(context.Background(), model.RegisterInput{Email: "a@example.com", Password: "password123"}); err != nil {
		t.Fatal(err)
	}
	if len(users.outbox) != 1 || users.outbox[0].Type != model.AuthEventUserRegistered || users.outbox[0].ID == "" {
		t.Fatalf("outbox = %+v", users.outbox)
	}
	if len(r.events.types) != 0 {
		t.Errorf("registered event was also published directly: %v", r.events.types)
	}
	// A duplicate registration must not leave a second event behind.
	_, _ = r.svc.Register(context.Background(), model.RegisterInput{Email: "a@example.com", Password: "password123"})
	if len(users.outbox) != 1 {
		t.Errorf("duplicate register added an outbox event: %d", len(users.outbox))
	}
}
