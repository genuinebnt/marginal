package authrest

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"marginal/api-gateway/internal/actorctx"
	"marginal/api-gateway/internal/apierror"
	authv1 "marginal/auth-service/genproto/authv1"
)

// Handler holds the one thing every route needs: an AuthService client.
// authv1.AuthServiceClient is already exactly the six methods this
// package calls — AuthService has no other RPCs.
type Handler struct {
	client authv1.AuthServiceClient
}

func NewHandler(client authv1.AuthServiceClient) *Handler { return &Handler{client: client} }

// Mount registers auth.md §2's six routes on r.
func (h *Handler) Mount(r chi.Router) {
	r.Post("/auth/register", h.register)
	r.Post("/auth/login", h.login)
	r.Get("/auth/users/{id}", h.getUser)
	r.Post("/auth/refresh", h.refresh)
	r.Post("/auth/revoke", h.revoke)
	r.Post("/auth/revoke-all", h.revokeAll)
	// § 18 ADMIN. Under /admin rather than /auth because it is a
	// workspace view, not an authentication operation — and it is
	// NOT authorization-gated, because RBAC is v3.1.0 and there is
	// nothing to gate on yet. The screen says so.
	r.Get("/admin/people", h.listPeople)
}

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var body registerRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.WriteBadRequest(w, "invalid JSON body")
		return
	}

	tokens, err := h.client.Register(actorctx.FromRequest(r), &authv1.RegisterRequest{
		Email:       body.Email,
		Password:    body.Password,
		DisplayName: body.DisplayName,
	})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusCreated, toTokenPairJSON(tokens))
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body loginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.WriteBadRequest(w, "invalid JSON body")
		return
	}

	tokens, err := h.client.Authenticate(actorctx.FromRequest(r), &authv1.AuthenticateRequest{
		Email:    body.Email,
		Password: body.Password,
	})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toTokenPairJSON(tokens))
}

func (h *Handler) getUser(w http.ResponseWriter, r *http.Request) {
	user, err := h.client.GetUser(actorctx.FromRequest(r), &authv1.GetUserRequest{Id: chi.URLParam(r, "id")})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toUserJSON(user))
}

func (h *Handler) listPeople(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.ListPeople(actorctx.FromRequest(r), &authv1.ListPeopleRequest{})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toPeopleJSON(resp))
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var body refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.WriteBadRequest(w, "invalid JSON body")
		return
	}

	tokens, err := h.client.Refresh(actorctx.FromRequest(r), &authv1.RefreshRequest{RefreshToken: body.RefreshToken})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toTokenPairJSON(tokens))
}

type revokeRequest struct {
	RefreshToken string  `json:"refresh_token"`
	AccessToken  *string `json:"access_token,omitempty"`
}

func (h *Handler) revoke(w http.ResponseWriter, r *http.Request) {
	var body revokeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.WriteBadRequest(w, "invalid JSON body")
		return
	}

	_, err := h.client.Revoke(actorctx.FromRequest(r), &authv1.RevokeRequest{
		RefreshToken: body.RefreshToken,
		AccessToken:  body.AccessToken,
	})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) revokeAll(w http.ResponseWriter, r *http.Request) {
	_, err := h.client.RevokeAll(actorctx.FromRequest(r), &authv1.RevokeAllRequest{})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
