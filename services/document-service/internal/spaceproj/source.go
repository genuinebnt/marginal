package spaceproj

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	authv1 "marginal/auth-service/genproto/authv1"
)

// GRPCSource asks auth-service for the whole membership set.
//
// This is the only place document-service talks to auth-service, and it is
// deliberately NOT on any request path: it runs at startup and on a timer.
// A per-request call would put a second hop in front of every read and make
// listing pages depend on auth-service being up — the same argument
// marginal/authverify makes about verifying tokens locally.
type GRPCSource struct {
	client authv1.SpaceServiceClient
}

func NewGRPCSource(conn grpc.ClientConnInterface) *GRPCSource {
	return &GRPCSource{client: authv1.NewSpaceServiceClient(conn)}
}

func (s *GRPCSource) ListAllMemberships(ctx context.Context) ([]Membership, error) {
	resp, err := s.client.ListAllMemberships(ctx, &authv1.ListAllMembershipsRequest{})
	if err != nil {
		return nil, fmt.Errorf("spaceproj: ListAllMemberships: %w", err)
	}
	out := make([]Membership, 0, len(resp.GetMemberships()))
	for _, m := range resp.GetMemberships() {
		user, err := uuid.Parse(m.GetUserId())
		if err != nil {
			continue
		}
		space, err := uuid.Parse(m.GetSpaceId())
		if err != nil {
			continue
		}
		out = append(out, Membership{
			UserID: user, SpaceID: space, Role: m.GetRole(),
			GrantedAt: m.GetCreatedAt().AsTime(),
		})
	}
	return out, nil
}
