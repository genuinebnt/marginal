// Package spacesrest is api-gateway's REST↔gRPC shim for SpaceService —
// docs/api/spaces.md §2. Same convention as graphrest and discoverrest:
// translation only, no decisions.
//
// In particular it does NOT enforce anything. The gateway authenticates and
// forwards; auth-service decides. That split is ADR-013 §4's, and it is why
// the status codes below are whatever the backend said rather than
// something this layer computes.
package spacesrest

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"marginal/api-gateway/internal/actorctx"
	"marginal/api-gateway/internal/apierror"
	authv1 "marginal/auth-service/genproto/authv1"
)

type Handler struct {
	client authv1.SpaceServiceClient
}

func NewHandler(client authv1.SpaceServiceClient) *Handler { return &Handler{client: client} }

func (h *Handler) Mount(r chi.Router) {
	r.Get("/spaces", h.list)
	r.Post("/spaces", h.create)
	r.Get("/spaces/{id}/members", h.members)
	r.Put("/spaces/{id}/members/{userId}", h.grant)
	r.Delete("/spaces/{id}/members/{userId}", h.revoke)
	// Invitations (docs/api/spaces.md § 5). POST to a space's invitations
	// rather than PUT to a membership, because it does not create one —
	// that is the whole distinction the feature rests on.
	r.Post("/spaces/{id}/invitations", h.invite)
	r.Get("/invitations", h.invitations)
	r.Post("/invitations/{id}/respond", h.respond)
}

type spaceJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
	CreatedBy string `json:"created_by"`
	// Not "role": the field answers "what may I do here", and a name that
	// reads as the space's own property invites a client to cache one
	// person's answer under a key that suggests it is everyone's.
	YourRole string `json:"your_role"`
	Members  int32  `json:"members"`
}

type memberJSON struct {
	UserID      string `json:"user_id"`
	SpaceID     string `json:"space_id"`
	Role        string `json:"role"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.ListSpaces(actorctx.FromRequest(r), &authv1.ListSpacesRequest{})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	out := make([]spaceJSON, 0, len(resp.GetSpaces()))
	for _, s := range resp.GetSpaces() {
		out = append(out, spaceJSON{
			ID: s.GetId(), Name: s.GetName(), IsDefault: s.GetIsDefault(),
			CreatedBy: s.GetCreatedBy(), YourRole: s.GetYourRole(), Members: s.GetMembers(),
		})
	}
	apierror.WriteJSON(w, http.StatusOK, map[string]any{"spaces": out})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.WriteBadRequest(w, "invalid JSON body")
		return
	}
	s, err := h.client.CreateSpace(actorctx.FromRequest(r), &authv1.CreateSpaceRequest{Name: body.Name})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusCreated, spaceJSON{
		ID: s.GetId(), Name: s.GetName(), IsDefault: s.GetIsDefault(),
		CreatedBy: s.GetCreatedBy(), YourRole: s.GetYourRole(), Members: s.GetMembers(),
	})
}

func (h *Handler) members(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.ListMembers(actorctx.FromRequest(r),
		&authv1.ListMembersRequest{SpaceId: chi.URLParam(r, "id")})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	out := make([]memberJSON, 0, len(resp.GetMembers()))
	for _, m := range resp.GetMembers() {
		out = append(out, memberJSON{
			UserID: m.GetUserId(), SpaceID: m.GetSpaceId(), Role: m.GetRole(),
			DisplayName: m.GetDisplayName(), Email: m.GetEmail(),
		})
	}
	apierror.WriteJSON(w, http.StatusOK, map[string]any{"members": out})
}

func (h *Handler) grant(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.WriteBadRequest(w, "invalid JSON body")
		return
	}
	m, err := h.client.GrantRole(actorctx.FromRequest(r), &authv1.GrantRoleRequest{
		SpaceId: chi.URLParam(r, "id"), UserId: chi.URLParam(r, "userId"), Role: body.Role,
	})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, memberJSON{
		UserID: m.GetUserId(), SpaceID: m.GetSpaceId(), Role: m.GetRole(),
	})
}

func (h *Handler) revoke(w http.ResponseWriter, r *http.Request) {
	if _, err := h.client.RevokeRole(actorctx.FromRequest(r), &authv1.RevokeRoleRequest{
		SpaceId: chi.URLParam(r, "id"), UserId: chi.URLParam(r, "userId"),
	}); err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type invitationJSON struct {
	ID            string `json:"id"`
	SpaceID       string `json:"space_id"`
	SpaceName     string `json:"space_name"`
	UserID        string `json:"user_id"`
	Role          string `json:"role"`
	InvitedBy     string `json:"invited_by"`
	InvitedByName string `json:"invited_by_name"`
	CreatedAt     string `json:"created_at"`
	RespondedAt   string `json:"responded_at,omitempty"`
	Accepted      bool   `json:"accepted"`
}

func invitationOf(i *authv1.Invitation) invitationJSON {
	out := invitationJSON{
		ID: i.GetId(), SpaceID: i.GetSpaceId(), SpaceName: i.GetSpaceName(),
		UserID: i.GetUserId(), Role: i.GetRole(), InvitedBy: i.GetInvitedBy(),
		InvitedByName: i.GetInvitedByName(), Accepted: i.GetAccepted(),
	}
	if t := i.GetCreatedAt(); t != nil {
		out.CreatedAt = t.AsTime().UTC().Format(time.RFC3339)
	}
	if t := i.GetRespondedAt(); t != nil {
		out.RespondedAt = t.AsTime().UTC().Format(time.RFC3339)
	}
	return out
}

func (h *Handler) invite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.WriteBadRequest(w, "invalid JSON body")
		return
	}
	inv, err := h.client.InviteMember(actorctx.FromRequest(r), &authv1.InviteMemberRequest{
		SpaceId: chi.URLParam(r, "id"), UserId: body.UserID, Role: body.Role,
	})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusCreated, invitationOf(inv))
}

func (h *Handler) invitations(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.ListInvitations(actorctx.FromRequest(r), &authv1.ListInvitationsRequest{})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	out := make([]invitationJSON, 0, len(resp.GetInvitations()))
	for _, i := range resp.GetInvitations() {
		out = append(out, invitationOf(i))
	}
	apierror.WriteJSON(w, http.StatusOK, map[string]any{"invitations": out})
}

func (h *Handler) respond(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Accept bool `json:"accept"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.WriteBadRequest(w, "invalid JSON body")
		return
	}
	inv, err := h.client.RespondToInvitation(actorctx.FromRequest(r), &authv1.RespondToInvitationRequest{
		InvitationId: chi.URLParam(r, "id"), Accept: body.Accept,
	})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, invitationOf(inv))
}
