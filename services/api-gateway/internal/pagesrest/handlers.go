package pagesrest

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"marginal/api-gateway/internal/actorctx"
	"marginal/api-gateway/internal/apierror"
	documentv1 "marginal/document-service/genproto/documentv1"
)

// Handler holds the one thing every route needs: a PageService client.
// documentv1.PageServiceClient is already exactly the RPCs this package
// calls — PageService has no other consumers — so it doubles as the
// "small interface at the point of use" (CLOUD_PORTABILITY.md) without a
// redundant hand-written duplicate.
type Handler struct {
	client documentv1.PageServiceClient
}

func NewHandler(client documentv1.PageServiceClient) *Handler { return &Handler{client: client} }

// Mount registers pages.md §2's six routes on r, plus backlinks (not in
// pages.md §2's original six — internal/blockproj's projection, not page
// metadata; see ListBacklinks's own doc comment on PageService).
func (h *Handler) Mount(r chi.Router) {
	r.Post("/pages", h.create)
	r.Get("/pages", h.list)
	r.Get("/pages/{id}", h.get)
	r.Patch("/pages/{id}/title", h.rename)
	r.Patch("/pages/{id}/parent", h.reparent)
	r.Delete("/pages/{id}", h.delete)
	r.Get("/pages/{id}/backlinks", h.backlinks)
}

type createPageRequest struct {
	Title    string  `json:"title"`
	ParentID *string `json:"parent_id,omitempty"`
	After    *string `json:"after,omitempty"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var body createPageRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.WriteBadRequest(w, "invalid JSON body")
		return
	}

	page, err := h.client.CreatePage(actorctx.FromRequest(r), &documentv1.CreatePageRequest{
		Title:    body.Title,
		ParentId: body.ParentID,
		After:    body.After,
	})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toPageJSON(page))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	page, err := h.client.GetPage(actorctx.FromRequest(r), &documentv1.GetPageRequest{Id: id})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toPageJSON(page))
}

func (h *Handler) backlinks(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	resp, err := h.client.ListBacklinks(actorctx.FromRequest(r), &documentv1.ListBacklinksRequest{PageId: id})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toListBacklinksJSON(resp))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	req := &documentv1.ListPagesRequest{}
	if v := q.Get("parent_id"); v != "" {
		req.ParentId = &v
	}
	if v := q.Get("after"); v != "" {
		req.After = &v
	}
	if v := q.Get("limit"); v != "" {
		limit, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			apierror.WriteBadRequest(w, "limit must be an integer")
			return
		}
		l := int32(limit)
		req.Limit = &l
	}

	resp, err := h.client.ListPages(actorctx.FromRequest(r), req)
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toListPagesJSON(resp))
}

type renamePageRequest struct {
	Title string `json:"title"`
}

func (h *Handler) rename(w http.ResponseWriter, r *http.Request) {
	var body renamePageRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.WriteBadRequest(w, "invalid JSON body")
		return
	}

	page, err := h.client.RenamePage(actorctx.FromRequest(r), &documentv1.RenamePageRequest{
		Id:    chi.URLParam(r, "id"),
		Title: body.Title,
	})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toPageJSON(page))
}

type reparentPageRequest struct {
	ParentID *string `json:"parent_id"`
	After    *string `json:"after,omitempty"`
}

func (h *Handler) reparent(w http.ResponseWriter, r *http.Request) {
	var body reparentPageRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.WriteBadRequest(w, "invalid JSON body")
		return
	}

	page, err := h.client.ReparentPage(actorctx.FromRequest(r), &documentv1.ReparentPageRequest{
		Id:       chi.URLParam(r, "id"),
		ParentId: body.ParentID,
		After:    body.After,
	})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toPageJSON(page))
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	_, err := h.client.DeletePage(actorctx.FromRequest(r), &documentv1.DeletePageRequest{Id: chi.URLParam(r, "id")})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
