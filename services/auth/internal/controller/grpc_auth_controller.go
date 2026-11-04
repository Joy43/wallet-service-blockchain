package controller

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/yourorg/wallet-service/gen/auth/v1"
	"github.com/yourorg/wallet-service/services/auth/internal/model"
)

// TokenValidator is the use case the gRPC controller exposes.
type TokenValidator interface {
	ValidateToken(ctx context.Context, token string) (model.Claims, error)
}

type GRPCAuthController struct {
	authv1.UnimplementedAuthServiceServer
	svc TokenValidator
}

func NewGRPCAuthController(svc TokenValidator) *GRPCAuthController {
	return &GRPCAuthController{svc: svc}
}

func (s *GRPCAuthController) ValidateToken(ctx context.Context, req *authv1.ValidateTokenRequest) (*authv1.ValidateTokenResponse, error) {
	c, err := s.svc.ValidateToken(ctx, req.GetAccessToken())
	if err != nil {
		return nil, toStatus(err)
	}
	return &authv1.ValidateTokenResponse{
		UserId: c.UserID, Role: c.Role, SessionId: c.SessionID, ExpiresAtUnix: c.ExpiresAt.Unix(),
	}, nil
}

// toStatus is the ONLY place domain codes become gRPC codes.
func toStatus(err error) error {
	switch model.CodeOf(err) {
	case model.ErrUnauthenticated:
		return status.Error(codes.Unauthenticated, model.MessageOf(err))
	case model.ErrForbidden:
		return status.Error(codes.PermissionDenied, model.MessageOf(err))
	case model.ErrInvalid:
		return status.Error(codes.InvalidArgument, model.MessageOf(err))
	case model.ErrUnavailable:
		return status.Error(codes.Unavailable, model.MessageOf(err))
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
