// Package api implements documentv1... (authv1) AuthServiceServer over
// authservice.Service — proto <-> domain translation and domain errors <->
// gRPC status codes (docs/api/auth.md § Status codes). No business logic
// of its own.
package api

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	authv1 "marginal/auth-service/genproto/authv1"
	"marginal/auth-service/internal/authservice"
	"marginal/auth-service/internal/domain"
	"marginal/auth-service/internal/users"
)

type Server struct {
	authv1.UnimplementedAuthServiceServer
	Service *authservice.Service
}

func (s *Server) Register(ctx context.Context, req *authv1.RegisterRequest) (*authv1.TokenPair, error) {
	email, err := domain.NewEmail(req.GetEmail())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	password, err := domain.NewPassword(req.GetPassword())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	displayName, err := domain.NewDisplayName(req.GetDisplayName())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	_, pair, err := s.Service.Register(ctx, email, password, displayName)
	if err != nil {
		return nil, toStatus(err)
	}
	return toProtoTokenPair(pair), nil
}

// Authenticate deliberately doesn't strictly validate the email's syntax
// the way Register does — a malformed address on login just falls
// through the same "unknown email" (dummy-hash, constant-time) path as a
// well-formed but unregistered one, rather than becoming a distinct
// INVALID_ARGUMENT case that would need its own timing story.
func (s *Server) Authenticate(ctx context.Context, req *authv1.AuthenticateRequest) (*authv1.TokenPair, error) {
	email := domain.EmailFromStored(strings.ToLower(strings.TrimSpace(req.GetEmail())))
	password, err := domain.NewPassword(req.GetPassword())
	if err != nil {
		// A password that fails basic length checks can't possibly match
		// a stored hash either way — safe to reject before touching the
		// dummy-hash path, no credential-oracle risk (this rejects
		// purely on shape, identically for every account or none).
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	_, pair, err := s.Service.Authenticate(ctx, email, password)
	if err != nil {
		return nil, toStatus(err)
	}
	return toProtoTokenPair(pair), nil
}

func (s *Server) GetUser(ctx context.Context, req *authv1.GetUserRequest) (*authv1.User, error) {
	id, err := domain.ParseUserID(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid id")
	}
	user, err := s.Service.GetUser(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return toProtoUser(user), nil
}

func (s *Server) Refresh(ctx context.Context, req *authv1.RefreshRequest) (*authv1.TokenPair, error) {
	pair, err := s.Service.Refresh(ctx, req.GetRefreshToken())
	if err != nil {
		return nil, toStatus(err)
	}
	return toProtoTokenPair(pair), nil
}

func (s *Server) Revoke(ctx context.Context, req *authv1.RevokeRequest) (*emptypb.Empty, error) {
	if err := s.Service.Revoke(ctx, req.GetRefreshToken(), req.AccessToken); err != nil {
		return nil, toStatus(err)
	}
	return &emptypb.Empty{}, nil
}

// RevokeAll's target is always the caller's own sessions, read from
// actor-id metadata — same temporary stand-in document-service's pages
// package uses (docs/api/auth.md § Actor identity); replace it once a
// real api-gateway exists.
func (s *Server) RevokeAll(ctx context.Context, _ *authv1.RevokeAllRequest) (*emptypb.Empty, error) {
	userID, err := actorID(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.Service.RevokeAll(ctx, userID); err != nil {
		return nil, toStatus(err)
	}
	return &emptypb.Empty{}, nil
}

func actorID(ctx context.Context) (domain.UserID, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return domain.UserID{}, status.Error(codes.Unauthenticated, "missing actor-id")
	}
	values := md.Get("actor-id")
	if len(values) == 0 || values[0] == "" {
		return domain.UserID{}, status.Error(codes.Unauthenticated, "missing actor-id")
	}
	id, err := domain.ParseUserID(values[0])
	if err != nil {
		return domain.UserID{}, status.Error(codes.Unauthenticated, "invalid actor-id")
	}
	return id, nil
}

func toStatus(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, authservice.ErrInvalidCredentials):
		return status.Error(codes.Unauthenticated, "invalid credentials")
	case errors.Is(err, authservice.ErrSessionExpired):
		return status.Error(codes.Unauthenticated, "session expired")
	case errors.Is(err, users.ErrNotFound):
		return status.Error(codes.NotFound, "user not found")
	case errors.Is(err, users.ErrEmailTaken):
		// A real, live path now that Register is ordinary repeatable
		// signup, not a one-time bootstrap claim (docs/porting/PROGRESS.md).
		return status.Error(codes.AlreadyExists, "email already registered")
	default:
		var st interface{ GRPCStatus() *status.Status }
		if errors.As(err, &st) {
			return err
		}
		// auth.md's status table says every INTERNAL is "logged as: error"
		// — the client only ever sees "internal error", no detail, but
		// this is the one place the real cause isn't lost entirely.
		slog.Error("auth-service: unmapped internal error", "err", err)
		return status.Error(codes.Internal, "internal error")
	}
}

func toProtoTokenPair(p authservice.TokenPair) *authv1.TokenPair {
	return &authv1.TokenPair{
		AccessToken:  p.AccessToken,
		RefreshToken: p.RefreshToken,
		ExpiresIn:    p.ExpiresIn,
	}
}

func toProtoUser(u users.User) *authv1.User {
	return &authv1.User{
		Id:          u.ID.String(),
		Email:       u.Email.String(),
		DisplayName: u.DisplayName.String(),
		CursorColor: u.CursorColor.String(),
		CreatedAt:   timestamppb.New(u.CreatedAt),
	}
}
