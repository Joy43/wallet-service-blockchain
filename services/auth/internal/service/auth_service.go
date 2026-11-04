package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/yourorg/wallet-service/services/auth/internal/model"
)

// ErrTokenReuse is returned by SessionStore.Rotate when a rotated-out refresh
// token is presented again — a strong sign the token was stolen.
var ErrTokenReuse = errors.New("refresh token reuse detected")

type AuthConfig struct {
	RefreshTTL   time.Duration
	UserCacheTTL time.Duration
	LoginLimit   int // attempts allowed per LoginWindow per email
	LoginWindow  time.Duration
}

type AuthService struct {
	users    UserRepository
	cache    UserCache
	sessions SessionStore
	limiter  RateLimiter
	tokens   TokenIssuer
	hasher   PasswordHasher
	events   AuthEventPublisher
	cfg      AuthConfig
	log      *slog.Logger
	now      func() time.Time
	// dummyHash is compared against when the email is unknown so login timing
	// does not reveal which emails are registered.
	dummyHash string
}

func NewAuthService(users UserRepository, cache UserCache, sessions SessionStore, limiter RateLimiter,
	tokens TokenIssuer, hasher PasswordHasher, events AuthEventPublisher, cfg AuthConfig, log *slog.Logger) (*AuthService, error) {
	dummy, err := hasher.Hash("dummy-password-for-timing")
	if err != nil {
		return nil, fmt.Errorf("prepare dummy hash: %w", err)
	}
	return &AuthService{users: users, cache: cache, sessions: sessions, limiter: limiter, tokens: tokens,
		hasher: hasher, events: events, cfg: cfg, log: log, now: time.Now, dummyHash: dummy}, nil
}

func (s *AuthService) Register(ctx context.Context, in model.RegisterInput) (model.User, error) {
	if err := in.Validate(); err != nil {
		return model.User{}, err
	}
	hash, err := s.hasher.Hash(in.Password)
	if err != nil {
		return model.User{}, fmt.Errorf("hash password: %w", err)
	}
	u := model.User{ID: randomToken(12), Email: in.Email, PasswordHash: hash, Role: model.RoleUser, CreatedAt: s.now().UTC()}
	// The registered event goes through the outbox, in the same DB transaction as the user.
	ev := model.AuthEvent{ID: randomToken(12), Type: model.AuthEventUserRegistered, UserID: u.ID, Email: u.Email, OccurredAt: u.CreatedAt}
	if err := s.users.CreateWithEvent(ctx, u, ev); err != nil {
		return model.User{}, err // already a *model.Error (ErrConflict) for duplicates
	}
	return u, nil
}

func (s *AuthService) Login(ctx context.Context, in model.LoginInput) (model.TokenPair, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))

	ok, err := s.limiter.Allow(ctx, "login:"+email, s.cfg.LoginLimit, s.cfg.LoginWindow)
	if err != nil {
		// Fail open on limiter outage: Redis down must not lock everyone out.
		s.log.WarnContext(ctx, "rate limiter unavailable", slog.Any("error", err))
	} else if !ok {
		return model.TokenPair{}, model.NewError(model.ErrRateLimited, "too many login attempts, try again later")
	}

	u, err := s.users.GetByEmail(ctx, email)
	switch {
	case model.CodeOf(err) == model.ErrNotFound:
		_ = s.hasher.Compare(s.dummyHash, in.Password)
		return model.TokenPair{}, s.loginFailed(ctx, email, in.IP)
	case err != nil:
		return model.TokenPair{}, fmt.Errorf("get user: %w", err)
	}
	if s.hasher.Compare(u.PasswordHash, in.Password) != nil {
		return model.TokenPair{}, s.loginFailed(ctx, email, in.IP)
	}

	pair, err := s.startSession(ctx, u)
	if err != nil {
		return model.TokenPair{}, err
	}
	s.publish(ctx, model.AuthEventUserLoggedIn, u.ID, u.Email, in.IP)
	return pair, nil
}

func (s *AuthService) loginFailed(ctx context.Context, email, ip string) error {
	s.publish(ctx, model.AuthEventLoginFailed, "", email, ip)
	// Same message for unknown email and wrong password.
	return model.NewError(model.ErrUnauthenticated, "invalid email or password")
}

// Refresh exchanges a refresh token for a new pair and rotates the refresh secret.
// Presenting an already-rotated token revokes the whole session.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (model.TokenPair, error) {
	sid, secret, ok := strings.Cut(refreshToken, ".")
	if !ok || sid == "" || secret == "" {
		return model.TokenPair{}, model.NewError(model.ErrUnauthenticated, "invalid refresh token")
	}
	newSecret := randomToken(32)
	sess, err := s.sessions.Rotate(ctx, sid, hashSecret(secret), hashSecret(newSecret), s.cfg.RefreshTTL)
	switch {
	case errors.Is(err, ErrTokenReuse):
		_ = s.sessions.Revoke(ctx, sid)
		s.publish(ctx, model.AuthEventTokenReuse, "", "", "")
		return model.TokenPair{}, model.NewError(model.ErrUnauthenticated, "invalid refresh token")
	case err != nil:
		if model.CodeOf(err) == model.ErrUnauthenticated {
			return model.TokenPair{}, model.NewError(model.ErrUnauthenticated, "invalid refresh token")
		}
		return model.TokenPair{}, fmt.Errorf("rotate session: %w", err)
	}
	access, accessExp, err := s.tokens.Issue(sess.UserID, sess.Role, sess.ID)
	if err != nil {
		return model.TokenPair{}, fmt.Errorf("issue access token: %w", err)
	}
	return model.TokenPair{
		AccessToken: access, AccessExpiresAt: accessExp,
		RefreshToken: sess.ID + "." + newSecret, RefreshExpiresAt: s.now().Add(s.cfg.RefreshTTL).UTC(),
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, c model.Claims) error {
	if err := s.sessions.Revoke(ctx, c.SessionID); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	s.publish(ctx, model.AuthEventUserLoggedOut, c.UserID, "", "")
	return nil
}

// ValidateToken is the hot path called by every protected request (HTTP and gRPC):
// verify the JWT signature/expiry, then confirm the session was not revoked.
func (s *AuthService) ValidateToken(ctx context.Context, token string) (model.Claims, error) {
	c, err := s.tokens.Parse(token)
	if err != nil {
		return model.Claims{}, model.NewError(model.ErrUnauthenticated, "invalid or expired token")
	}
	active, err := s.sessions.Active(ctx, c.SessionID)
	if err != nil {
		return model.Claims{}, model.NewError(model.ErrUnavailable, "session store unavailable")
	}
	if !active {
		return model.Claims{}, model.NewError(model.ErrUnauthenticated, "session has ended")
	}
	return c, nil
}

// PublicKeys returns the keys other services use to verify access tokens (JWKS).
func (s *AuthService) PublicKeys() []model.PublicKey { return s.tokens.PublicKeys() }

// Me returns the profile for a user id, cache-aside through Redis.
func (s *AuthService) Me(ctx context.Context, userID string) (model.User, error) {
	if u, hit, err := s.cache.Get(ctx, userID); err != nil {
		s.log.WarnContext(ctx, "user cache read failed", slog.Any("error", err))
	} else if hit {
		return u, nil
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return model.User{}, err
	}
	if err := s.cache.Set(ctx, u, s.cfg.UserCacheTTL); err != nil {
		s.log.WarnContext(ctx, "user cache write failed", slog.Any("error", err))
	}
	return u, nil
}

func (s *AuthService) startSession(ctx context.Context, u model.User) (model.TokenPair, error) {
	sid, secret := randomToken(16), randomToken(32)
	if err := s.sessions.Create(ctx, model.Session{ID: sid, UserID: u.ID, Role: u.Role, SecretHash: hashSecret(secret)}, s.cfg.RefreshTTL); err != nil {
		return model.TokenPair{}, fmt.Errorf("create session: %w", err)
	}
	access, accessExp, err := s.tokens.Issue(u.ID, u.Role, sid)
	if err != nil {
		return model.TokenPair{}, fmt.Errorf("issue access token: %w", err)
	}
	return model.TokenPair{
		AccessToken: access, AccessExpiresAt: accessExp,
		RefreshToken: sid + "." + secret, RefreshExpiresAt: s.now().Add(s.cfg.RefreshTTL).UTC(),
	}, nil
}

// publish is best-effort: auditing must never fail a login. (See README: use an outbox for guaranteed delivery.)
func (s *AuthService) publish(ctx context.Context, typ, userID, email, ip string) {
	e := model.AuthEvent{ID: randomToken(12), Type: typ, UserID: userID, Email: email, IP: ip, OccurredAt: s.now().UTC()}
	if err := s.events.PublishAuthEvent(ctx, e); err != nil {
		s.log.WarnContext(ctx, "publish auth event failed", slog.String("type", typ), slog.Any("error", err))
	}
}

func randomToken(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashSecret(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}
