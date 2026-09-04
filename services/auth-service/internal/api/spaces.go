// SpaceService's gRPC surface — proto ↔ domain translation and nothing
// else. Every rule is internal/spaces'; this file's only judgement is how
// each error becomes a status code, and that mapping is itself
// security-relevant (see toSpaceStatus).
package api

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	authv1 "marginal/auth-service/genproto/authv1"
	authrepo "marginal/auth-service/internal/authrepo/gen"
	"marginal/auth-service/internal/spaces"
)

type SpaceServer struct {
	authv1.UnimplementedSpaceServiceServer
	q   *authrepo.Queries
	svc *spaces.Service
}

func NewSpaceServer(pool *pgxpool.Pool) *SpaceServer {
	return &SpaceServer{q: authrepo.New(pool), svc: spaces.NewService(pool)}
}

func uuidOf(p pgtype.UUID) uuid.UUID { return uuid.UUID(p.Bytes) }

func (s *SpaceServer) ListSpaces(ctx context.Context, _ *authv1.ListSpacesRequest) (*authv1.ListSpacesResponse, error) {
	caller, err := actorID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.q.ListSpacesForUser(ctx, pgtype.UUID{Bytes: uuid.UUID(caller), Valid: true})
	if err != nil {
		return nil, status.Error(codes.Internal, "spaces: listing failed")
	}
	out := &authv1.ListSpacesResponse{Spaces: make([]*authv1.Space, 0, len(rows))}
	for _, r := range rows {
		out.Spaces = append(out.Spaces, &authv1.Space{
			Id: uuidOf(r.ID).String(), Name: r.Name, IsDefault: r.IsDefault,
			CreatedBy: uuidOf(r.CreatedBy).String(),
			CreatedAt: timestamppb.New(r.CreatedAt.Time),
			YourRole:  r.YourRole, Members: int32(r.Members),
		})
	}
	return out, nil
}

func (s *SpaceServer) CreateSpace(ctx context.Context, req *authv1.CreateSpaceRequest) (*authv1.Space, error) {
	caller, err := actorID(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "spaces: name is required")
	}
	id, err := s.svc.CreateSpace(ctx, uuid.UUID(caller), req.GetName())
	if err != nil {
		return nil, toSpaceStatus(err)
	}
	return &authv1.Space{
		Id: id.String(), Name: req.GetName(), CreatedBy: uuid.UUID(caller).String(),
		// The creator is always its admin — a space without one is a space
		// nobody can ever administer.
		YourRole: string(spaces.Admin), Members: 1,
	}, nil
}

func (s *SpaceServer) ListMembers(ctx context.Context, req *authv1.ListMembersRequest) (*authv1.ListMembersResponse, error) {
	caller, spaceID, err := s.callerAndSpace(ctx, req.GetSpaceId())
	if err != nil {
		return nil, err
	}
	// Admin-only, and a non-member gets NOT_FOUND rather than a 403 that
	// would confirm the space exists.
	if _, err := spaces.Authorize(ctx, spaces.NewPostgresStore(s.q), caller, spaceID, spaces.Admin); err != nil {
		return nil, toSpaceStatus(err)
	}
	rows, err := s.q.ListMembers(ctx, pgtype.UUID{Bytes: spaceID, Valid: true})
	if err != nil {
		return nil, status.Error(codes.Internal, "spaces: listing members failed")
	}
	out := &authv1.ListMembersResponse{Members: make([]*authv1.Membership, 0, len(rows))}
	for _, r := range rows {
		out.Members = append(out.Members, &authv1.Membership{
			UserId: uuidOf(r.UserID).String(), SpaceId: uuidOf(r.SpaceID).String(),
			Role: r.Role, DisplayName: r.DisplayName, Email: r.Email,
			CreatedAt: timestamppb.New(r.CreatedAt.Time),
		})
	}
	return out, nil
}

func (s *SpaceServer) GrantRole(ctx context.Context, req *authv1.GrantRoleRequest) (*authv1.Membership, error) {
	caller, spaceID, err := s.callerAndSpace(ctx, req.GetSpaceId())
	if err != nil {
		return nil, err
	}
	target, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "spaces: invalid user_id")
	}
	if err := s.svc.Grant(ctx, caller, spaceID, target, spaces.Role(req.GetRole())); err != nil {
		return nil, toSpaceStatus(err)
	}
	return &authv1.Membership{
		UserId: target.String(), SpaceId: spaceID.String(), Role: req.GetRole(),
	}, nil
}

func (s *SpaceServer) RevokeRole(ctx context.Context, req *authv1.RevokeRoleRequest) (*emptypb.Empty, error) {
	caller, spaceID, err := s.callerAndSpace(ctx, req.GetSpaceId())
	if err != nil {
		return nil, err
	}
	target, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "spaces: invalid user_id")
	}
	if err := s.svc.Revoke(ctx, caller, spaceID, target); err != nil {
		return nil, toSpaceStatus(err)
	}
	return &emptypb.Empty{}, nil
}

// ListAllMemberships is document-service's periodic reconcile against this
// source of truth (DATA_MODEL.md § docs.space_members). It is not
// authorization-gated by SPACE, because there is no single space to gate
// it on — it is a service-to-service call, and the actor-id it carries is
// document-service's own. Worth naming as the one call here that answers
// across every space at once.
func (s *SpaceServer) ListAllMemberships(ctx context.Context, _ *authv1.ListAllMembershipsRequest) (*authv1.ListAllMembershipsResponse, error) {
	rows, err := s.q.ListAllMemberships(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, "spaces: listing memberships failed")
	}
	out := &authv1.ListAllMembershipsResponse{Memberships: make([]*authv1.Membership, 0, len(rows))}
	for _, r := range rows {
		out.Memberships = append(out.Memberships, &authv1.Membership{
			UserId: uuidOf(r.UserID).String(), SpaceId: uuidOf(r.SpaceID).String(),
			Role: r.Role, DisplayName: r.DisplayName, Email: r.Email,
			CreatedAt: timestamppb.New(r.CreatedAt.Time),
		})
	}
	return out, nil
}

func (s *SpaceServer) callerAndSpace(ctx context.Context, rawSpace string) (uuid.UUID, uuid.UUID, error) {
	caller, err := actorID(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	spaceID, err := uuid.Parse(rawSpace)
	if err != nil {
		return uuid.Nil, uuid.Nil, status.Error(codes.InvalidArgument, "spaces: invalid space_id")
	}
	return uuid.UUID(caller), spaceID, nil
}

// toSpaceStatus is the one place the 404-vs-403 decision is spelled out,
// and it is a security decision rather than a formatting one
// (docs/api/spaces.md §3).
//
// ErrNotAMember becomes NOT_FOUND: a 403 on a space you cannot see
// confirms it exists, and space names are chosen by people and often say
// what a team is working on. ErrInsufficientRole becomes PERMISSION_DENIED,
// because that caller is already a member and already knows.
func toSpaceStatus(err error) error {
	switch {
	case errors.Is(err, spaces.ErrNotAMember):
		return status.Error(codes.NotFound, "space not found")
	case errors.Is(err, spaces.ErrInsufficientRole):
		return status.Error(codes.PermissionDenied, "your role in this space does not allow that")
	case errors.Is(err, spaces.ErrLastAdmin):
		return status.Error(codes.FailedPrecondition, "a space must keep at least one admin")
	case errors.Is(err, spaces.ErrInvalidRole):
		return status.Error(codes.InvalidArgument, "role must be viewer, editor or admin")
	case errors.Is(err, spaces.ErrDefaultSpace):
		return status.Error(codes.FailedPrecondition, "the default space cannot be removed")
	case errors.Is(err, spaces.ErrAlreadyMember):
		return status.Error(codes.AlreadyExists, "they are already in this space")
	case errors.Is(err, spaces.ErrAlreadyInvited):
		return status.Error(codes.AlreadyExists, "they already have a pending invitation")
	case errors.Is(err, spaces.ErrAlreadyAnswered):
		// One code for "already answered", "not yours" and "no such
		// invitation" — telling them apart would confirm that somebody
		// else's invitation exists.
		return status.Error(codes.FailedPrecondition, "that invitation is not open to you")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}

// ── Invitations (v3.3.0) ─────────────────────────────────────────────────

func invitationPB(inv spaces.Invitation) *authv1.Invitation {
	out := &authv1.Invitation{
		Id: inv.ID.String(), SpaceId: inv.SpaceID.String(), SpaceName: inv.SpaceName,
		UserId: inv.UserID.String(), Role: string(inv.Role),
		InvitedBy: inv.InvitedBy.String(), InvitedByName: inv.InvitedByName,
		CreatedAt: timestamppb.New(inv.CreatedAt), Accepted: inv.Accepted,
	}
	if inv.RespondedAt != nil {
		out.RespondedAt = timestamppb.New(*inv.RespondedAt)
	}
	return out
}

func (s *SpaceServer) InviteMember(ctx context.Context, req *authv1.InviteMemberRequest) (*authv1.Invitation, error) {
	caller, spaceID, err := s.callerAndSpace(ctx, req.GetSpaceId())
	if err != nil {
		return nil, err
	}
	target, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "spaces: invalid user_id")
	}
	inv, err := s.svc.Invite(ctx, caller, spaceID, target, spaces.Role(req.GetRole()))
	if err != nil {
		return nil, toSpaceStatus(err)
	}
	return invitationPB(inv), nil
}

func (s *SpaceServer) RespondToInvitation(ctx context.Context, req *authv1.RespondToInvitationRequest) (*authv1.Invitation, error) {
	caller, err := actorID(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.GetInvitationId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "spaces: invalid invitation_id")
	}
	inv, err := s.svc.Respond(ctx, uuid.UUID(caller), id, req.GetAccept())
	if err != nil {
		return nil, toSpaceStatus(err)
	}
	return invitationPB(inv), nil
}

func (s *SpaceServer) ListInvitations(ctx context.Context, _ *authv1.ListInvitationsRequest) (*authv1.ListInvitationsResponse, error) {
	caller, err := actorID(ctx)
	if err != nil {
		return nil, err
	}
	// Scoped by the token, never by a parameter: a user_id argument here
	// would be a way to read somebody else's invitations.
	list, err := s.svc.PendingInvitations(ctx, uuid.UUID(caller))
	if err != nil {
		return nil, status.Error(codes.Internal, "spaces: listing invitations failed")
	}
	out := &authv1.ListInvitationsResponse{Invitations: make([]*authv1.Invitation, 0, len(list))}
	for _, inv := range list {
		out.Invitations = append(out.Invitations, invitationPB(inv))
	}
	return out, nil
}
