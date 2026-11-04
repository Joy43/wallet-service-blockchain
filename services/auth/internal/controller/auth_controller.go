package controller

import (
	"context"
	"log/slog"
	"net"
	"net/http"

	"github.com/yourorg/wallet-service/services/auth/internal/dto"
	"github.com/yourorg/wallet-service/services/auth/internal/httputil"
	"github.com/yourorg/wallet-service/services/auth/internal/middleware"
	"github.com/yourorg/wallet-service/services/auth/internal/model"
)

// AuthService is defined where it is used (consumer-side interface).
type AuthService interface {
	Register(ctx context.Context, in model.RegisterInput) (model.User, error)
	Login(ctx context.Context, in model.LoginInput) (model.TokenPair, error)
	Refresh(ctx context.Context, refreshToken string) (model.TokenPair, error)
	Logout(ctx context.Context, c model.Claims) error
	Me(ctx context.Context, userID string) (model.User, error)
	PublicKeys() []model.PublicKey
}

type AuthController struct {
	svc AuthService
	log *slog.Logger
}

func NewAuthController(svc AuthService, log *slog.Logger) *AuthController {
	return &AuthController{svc: svc, log: log}
}

const authServiceName = "auth-service"

func (c *AuthController) fail(w http.ResponseWriter, r *http.Request, err error) {
	httputil.WriteError(w, r, c.log, authServiceName, err)
}

func (c *AuthController) Register(w http.ResponseWriter, r *http.Request) {
	var req dto.CredentialsRequest
	if err := httputil.Decode(w, r, &req); err != nil {
		c.fail(w, r, err)
		return
	}
	u, err := c.svc.Register(r.Context(), model.RegisterInput{Email: req.Email, Password: req.Password})
	if err != nil {
		c.fail(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, dto.ToUserResponse(u))
}

func (c *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	var req dto.CredentialsRequest
	if err := httputil.Decode(w, r, &req); err != nil {
		c.fail(w, r, err)
		return
	}
	// RemoteAddr only: X-Forwarded-For is client-controlled unless a trusted proxy sets it.
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	pair, err := c.svc.Login(r.Context(), model.LoginInput{Email: req.Email, Password: req.Password, IP: ip})
	if err != nil {
		c.fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httputil.WriteJSON(w, http.StatusOK, dto.ToTokenResponse(pair))
}

func (c *AuthController) Refresh(w http.ResponseWriter, r *http.Request) {
	var req dto.RefreshRequest
	if err := httputil.Decode(w, r, &req); err != nil {
		c.fail(w, r, err)
		return
	}
	pair, err := c.svc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		c.fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httputil.WriteJSON(w, http.StatusOK, dto.ToTokenResponse(pair))
}

func (c *AuthController) Logout(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFrom(r.Context())
	if err := c.svc.Logout(r.Context(), claims); err != nil {
		c.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *AuthController) Me(w http.ResponseWriter, r *http.Request) {
	claims, _ := middleware.ClaimsFrom(r.Context())
	u, err := c.svc.Me(r.Context(), claims.UserID)
	if err != nil {
		c.fail(w, r, err)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, dto.ToUserResponse(u))
}

// JWKS publishes the public keys so other services can verify access tokens locally.
func (c *AuthController) JWKS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=300")
	httputil.WriteJSON(w, http.StatusOK, dto.ToJWKS(c.svc.PublicKeys()))
}
