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

	// v2.7.0 classification. /topics and /tags are top-level because they
	// are cross-page reads — nesting them under /pages would imply a page
	// scope neither has.
	r.Get("/topics", h.listTopics)
	r.Get("/tags", h.tagFacets)
	r.Put("/pages/{id}/topic", h.setTopic)
	r.Post("/pages/{id}/tags", h.addTag)
	r.Delete("/pages/{id}/tags/{tag}", h.removeTag)

	// v2.8.0 resume. /resume is top-level because it is a cross-page read
	// scoped to the CALLER, not to any one page.
	r.Get("/resume", h.listResume)

	// v2.9.0 — series. A series IS a page with children, so these read the
	// tree rather than a table of their own.
	// v2.6.0's delete saga, made visible (§ 23c). GET /trash lists it,
	// POST .../restore reverses the one step that is reversible.
	r.Get("/trash", h.listTrash)
	r.Get("/pages/{id}/delete-preview", h.previewDelete)
	r.Post("/pages/{id}/restore", h.restorePage)

	r.Get("/series", h.listSeries)
	r.Get("/pages/{id}/series", h.pageSeries)
	r.Put("/pages/{id}/position", h.savePosition)
}

type createPageRequest struct {
	Title    string  `json:"title"`
	ParentID *string `json:"parent_id,omitempty"`
	After    *string `json:"after,omitempty"`
	// Which space a root page lands in; absent means the default one.
	SpaceID *string `json:"space_id"`
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
		SpaceId:  body.SpaceID,
	})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusCreated, toPageJSON(page))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	page, err := h.client.GetPage(actorctx.FromRequest(r), &documentv1.GetPageRequest{Id: id})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toPageJSON(page))
}

func (h *Handler) backlinks(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	resp, err := h.client.ListBacklinks(actorctx.FromRequest(r), &documentv1.ListBacklinksRequest{PageId: id})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toListBacklinksJSON(resp))
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
	apierror.WriteJSON(w, http.StatusOK, toListPagesJSON(resp))
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
	apierror.WriteJSON(w, http.StatusOK, toPageJSON(page))
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
	apierror.WriteJSON(w, http.StatusOK, toPageJSON(page))
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	_, err := h.client.DeletePage(actorctx.FromRequest(r), &documentv1.DeletePageRequest{Id: chi.URLParam(r, "id")})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- v2.7.0 classification (docs/api/pages.md §2) -------------------------

type setTopicRequest struct {
	// Pointer so `{"topic_id": null}` — clearing the assignment back to
	// untopiced — is distinguishable from an absent field. Untopiced is a
	// real state, so clearing has to be expressible.
	TopicID *string `json:"topic_id"`
}

type tagRequest struct {
	Tag string `json:"tag"`
}

func (h *Handler) listTopics(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.ListTopics(actorctx.FromRequest(r), &documentv1.ListTopicsRequest{})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	topics := make([]*topicJSON, 0, len(resp.GetTopics()))
	for _, t := range resp.GetTopics() {
		topics = append(topics, toTopicJSON(t))
	}
	apierror.WriteJSON(w, http.StatusOK, map[string]any{
		"topics":          topics,
		"untopiced_pages": resp.GetUntopicedPages(),
	})
}

func (h *Handler) setTopic(w http.ResponseWriter, r *http.Request) {
	var body setTopicRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.WriteBadRequest(w, "invalid JSON body")
		return
	}
	page, err := h.client.SetPageTopic(actorctx.FromRequest(r), &documentv1.SetPageTopicRequest{
		PageId: chi.URLParam(r, "id"), TopicId: body.TopicID,
	})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toPageJSON(page))
}

func (h *Handler) addTag(w http.ResponseWriter, r *http.Request) {
	var body tagRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.WriteBadRequest(w, "invalid JSON body")
		return
	}
	page, err := h.client.AddPageTag(actorctx.FromRequest(r), &documentv1.AddPageTagRequest{
		PageId: chi.URLParam(r, "id"), Tag: body.Tag,
	})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toPageJSON(page))
}

// The tag travels in the path rather than a body: DELETE with a body is
// poorly supported by intermediaries and some fetch stacks, and a tag is
// already constrained to lowercase with no whitespace, so it is path-safe.
func (h *Handler) removeTag(w http.ResponseWriter, r *http.Request) {
	page, err := h.client.RemovePageTag(actorctx.FromRequest(r), &documentv1.RemovePageTagRequest{
		PageId: chi.URLParam(r, "id"), Tag: chi.URLParam(r, "tag"),
	})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toPageJSON(page))
}

func (h *Handler) tagFacets(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	resp, err := h.client.ListTagFacets(actorctx.FromRequest(r), &documentv1.ListTagFacetsRequest{
		Limit: int32(limit),
	})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	facets := make([]map[string]any, 0, len(resp.GetFacets()))
	for _, f := range resp.GetFacets() {
		facets = append(facets, map[string]any{
			"tag": f.GetTag(), "page_count": f.GetPageCount(), "topics_spanned": f.GetTopicsSpanned(),
		})
	}
	apierror.WriteJSON(w, http.StatusOK, map[string]any{"facets": facets})
}

// --- v2.8.0 resume -------------------------------------------------------

type savePositionRequest struct {
	// Pointer so an absent block is expressible: a page opened and scrolled
	// but never clicked into still has a position worth resuming to.
	BlockID    *string `json:"block_id"`
	CaretStart int32   `json:"caret_start"`
	CaretEnd   int32   `json:"caret_end"`
}

func (h *Handler) savePosition(w http.ResponseWriter, r *http.Request) {
	var body savePositionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		apierror.WriteBadRequest(w, "invalid JSON body")
		return
	}
	if _, err := h.client.SaveReadingPosition(actorctx.FromRequest(r), &documentv1.SaveReadingPositionRequest{
		PageId: chi.URLParam(r, "id"), BlockId: body.BlockID,
		CaretStart: body.CaretStart, CaretEnd: body.CaretEnd,
	}); err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listResume(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	resp, err := h.client.ListReadingPositions(actorctx.FromRequest(r), &documentv1.ListReadingPositionsRequest{
		Limit: int32(limit),
	})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	out := make([]map[string]any, 0, len(resp.GetPositions()))
	for _, p := range resp.GetPositions() {
		out = append(out, map[string]any{
			"page_id":     p.GetPageId(),
			"page_title":  p.GetPageTitle(),
			"block_id":    p.BlockId,
			"caret_start": p.GetCaretStart(),
			"caret_end":   p.GetCaretEnd(),
			"updated_at":  formatTimestamp(p.GetUpdatedAt()),
			"topic":       toTopicJSON(p.GetTopic()),
		})
	}
	apierror.WriteJSON(w, http.StatusOK, map[string]any{"positions": out})
}

func (h *Handler) listSeries(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.ListSeries(actorctx.FromRequest(r), &documentv1.ListSeriesRequest{})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toListSeriesJSON(resp))
}

func (h *Handler) pageSeries(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.GetPageSeries(actorctx.FromRequest(r),
		&documentv1.GetPageSeriesRequest{PageId: chi.URLParam(r, "id")})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toPageSeriesJSON(resp))
}

func (h *Handler) listTrash(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.ListTrash(actorctx.FromRequest(r), &documentv1.ListTrashRequest{})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toListTrashJSON(resp))
}

func (h *Handler) previewDelete(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.PreviewDelete(actorctx.FromRequest(r),
		&documentv1.PreviewDeleteRequest{Id: chi.URLParam(r, "id")})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toDeletePreviewJSON(resp))
}

func (h *Handler) restorePage(w http.ResponseWriter, r *http.Request) {
	resp, err := h.client.RestorePage(actorctx.FromRequest(r),
		&documentv1.RestorePageRequest{Id: chi.URLParam(r, "id")})
	if err != nil {
		apierror.WriteGRPCStatus(w, err)
		return
	}
	apierror.WriteJSON(w, http.StatusOK, toPageJSON(resp))
}
