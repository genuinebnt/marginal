package roles

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	authv1 "marginal/auth-service/genproto/authv1"
	documentv1 "marginal/document-service/genproto/documentv1"
)

// Resolver answers "what is this actor's role on this page" — once, at
// join. Two hops, because the answer spans two services: document-service
// owns which SPACE a page is in, auth-service owns who is in that space.
//
// Doing it here rather than at can_apply time is the whole point (see the
// package doc). Two calls on a WebSocket handshake is nothing; two calls
// per keystroke would be the design failure ADR-013 §3 exists to avoid.
type Resolver struct {
	pages  documentv1.PageServiceClient
	spaces authv1.SpaceServiceClient
}

func NewResolver(documentConn, authConn grpc.ClientConnInterface) *Resolver {
	return &Resolver{
		pages:  documentv1.NewPageServiceClient(documentConn),
		spaces: authv1.NewSpaceServiceClient(authConn),
	}
}

// Resolve returns the actor's role on the page, or "" if they have none.
//
// An error is NOT a role. Callers must treat a failure as "no", never as
// "assume the usual" — a resolver that fails open turns every outage in
// auth-service into an escalation for everyone connecting during it.
func (r *Resolver) Resolve(ctx context.Context, pageID, actorID uuid.UUID) (Role, error) {
	// withActor on BOTH hops. document-service rejects a call with no
	// actor-id, and the first version only set it on the second call —
	// which failed every join with UNAUTHENTICATED, correctly.
	page, err := r.pages.GetPage(withActor(ctx, actorID), &documentv1.GetPageRequest{Id: pageID.String()})
	if err != nil {
		return "", fmt.Errorf("roles: reading page's space: %w", err)
	}
	spaceID := page.GetSpaceId()
	if spaceID == "" {
		// A page with no space is a page no rule applies to, which
		// docs.pages makes impossible with a NOT NULL — so this means the
		// field did not survive the wire, and guessing would be worse than
		// refusing.
		return "", fmt.Errorf("roles: page %s reported no space", pageID)
	}

	// ListSpaces returns the spaces the CALLER is in, with their own role
	// — so the actor's identity travels as gRPC metadata and the answer is
	// about them rather than about anyone else.
	resp, err := r.spaces.ListSpaces(withActor(ctx, actorID), &authv1.ListSpacesRequest{})
	if err != nil {
		return "", fmt.Errorf("roles: listing actor's spaces: %w", err)
	}
	for _, s := range resp.GetSpaces() {
		if s.GetId() == spaceID {
			return Role(s.GetYourRole()), nil
		}
	}
	// Not a member of the page's space. An empty role, not an error: "you
	// are not in it" is an answer.
	return "", nil
}

// withActor attaches the actor's id as outgoing gRPC metadata — the key
// every service in this repo reads identity from (docs/api/auth.md
// § Actor identity). document-service rejects a call missing it.
func withActor(ctx context.Context, actorID uuid.UUID) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "actor-id", actorID.String())
}

// Member is one person in a page's space, as much of them as a mention
// needs: who to notify, and the name somebody would have typed.
type Member struct {
	UserID      uuid.UUID
	DisplayName string
}

// Members returns everyone in the space the page belongs to.
//
// The access check is GetPage, called AS THE ACTOR: document-service
// answers NOT_FOUND for a page outside the caller's spaces (v3.1.0), so an
// author who cannot see the page cannot reach anybody through it. That is
// the property mentions need — candidates are bounded by the page's own
// space — and it is enforced on the hop that can actually enforce it.
//
// The member list itself comes from ListAllMemberships, the internal
// reconcile call document-service's projection already uses, filtered to
// that space. NOT from ListMembers, which is ADMIN-ONLY: resolving through
// it meant only admins could mention anyone, and everybody else's @handle
// silently resolved to nobody. Found by trying it as a viewer, which is
// also the account most likely to be asking somebody a question.
func (r *Resolver) Members(ctx context.Context, pageID, actorID uuid.UUID) ([]Member, error) {
	page, err := r.pages.GetPage(withActor(ctx, actorID), &documentv1.GetPageRequest{Id: pageID.String()})
	if err != nil {
		return nil, fmt.Errorf("roles: reading page's space: %w", err)
	}
	spaceID := page.GetSpaceId()
	if spaceID == "" {
		return nil, fmt.Errorf("roles: page %s reported no space", pageID)
	}
	resp, err := r.spaces.ListAllMemberships(ctx, &authv1.ListAllMembershipsRequest{})
	if err != nil {
		return nil, fmt.Errorf("roles: listing memberships: %w", err)
	}
	var out []Member
	for _, m := range resp.GetMemberships() {
		if m.GetSpaceId() != spaceID {
			continue
		}
		id, err := uuid.Parse(m.GetUserId())
		if err != nil {
			// One unparseable id is not a reason to notify nobody.
			continue
		}
		out = append(out, Member{UserID: id, DisplayName: m.GetDisplayName()})
	}
	return out, nil
}
