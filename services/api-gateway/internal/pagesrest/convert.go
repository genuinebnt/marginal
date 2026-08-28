// Package pagesrest is pages.md §2's gateway REST mapping — a thin
// translation to/from document-service's PageService gRPC contract (§1 of
// that doc), which stays the source of truth for semantics.
package pagesrest

import (
	"time"

	documentv1 "marginal/document-service/genproto/documentv1"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// pageJSON matches pages.md §2's example body field-for-field, including
// deleted_at's omission while active (documented there explicitly: "sent
// as null" would be wrong).
type pageJSON struct {
	ID             string  `json:"id"`
	CreatedBy      string  `json:"created_by"`
	Title          string  `json:"title"`
	ParentID       *string `json:"parent_id"`
	Path           string  `json:"path"`
	SortKey        string  `json:"sort_key"`
	LifecycleState string  `json:"lifecycle_state"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	DeletedAt      *string `json:"deleted_at,omitempty"`
}

func toPageJSON(p *documentv1.Page) pageJSON {
	out := pageJSON{
		ID:             p.GetId(),
		CreatedBy:      p.GetCreatedBy(),
		Title:          p.GetTitle(),
		Path:           p.GetPath(),
		SortKey:        p.GetSortKey(),
		LifecycleState: lifecycleStateJSON(p.GetLifecycleState()),
		CreatedAt:      formatTimestamp(p.GetCreatedAt()),
		UpdatedAt:      formatTimestamp(p.GetUpdatedAt()),
	}
	if p.ParentId != nil {
		out.ParentID = p.ParentId
	}
	if p.DeletedAt != nil {
		s := formatTimestamp(p.DeletedAt)
		out.DeletedAt = &s
	}
	return out
}

// lifecycleStateJSON renders documentv1's proto enum as the lowercase
// word pages.md's example JSON uses, not the generated PascalCase
// LIFECYCLE_STATE_* constant name.
func lifecycleStateJSON(s documentv1.LifecycleState) string {
	switch s {
	case documentv1.LifecycleState_LIFECYCLE_STATE_ACTIVE:
		return "active"
	case documentv1.LifecycleState_LIFECYCLE_STATE_DELETING:
		return "deleting"
	case documentv1.LifecycleState_LIFECYCLE_STATE_DELETED:
		return "deleted"
	default:
		return "unspecified"
	}
}

// formatTimestamp is pages.md §2's "RFC 3339 UTC — google.protobuf.Timestamp
// on the wire, formatted at the gateway."
func formatTimestamp(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.AsTime().UTC().Format(time.RFC3339Nano)
}

// backlinkJSON is one row of GET /pages/{id}/backlinks — deliberately not
// a pageJSON: a backlink only ever needs enough to render a row in the
// inspector (docs/ui-mockups/v2/index.html § 04 EDITOR's Backlinks tab), not a page's
// full metadata.
type backlinkJSON struct {
	FromPage        string `json:"from_page"`
	FromPageTitle   string `json:"from_page_title"`
	FromPageDeleted bool   `json:"from_page_deleted"`
	TargetTitle     string `json:"target_title"`
}

type listBacklinksJSON struct {
	Backlinks []backlinkJSON `json:"backlinks"`
}

func toListBacklinksJSON(resp *documentv1.ListBacklinksResponse) listBacklinksJSON {
	out := make([]backlinkJSON, len(resp.GetBacklinks()))
	for i, b := range resp.GetBacklinks() {
		out[i] = backlinkJSON{
			FromPage:        b.GetFromPage(),
			FromPageTitle:   b.GetFromPageTitle(),
			FromPageDeleted: b.GetFromPageDeleted(),
			TargetTitle:     b.GetTargetTitle(),
		}
	}
	return listBacklinksJSON{Backlinks: out}
}

type listPagesJSON struct {
	Pages      []pageJSON `json:"pages"`
	NextCursor *string    `json:"next_cursor,omitempty"`
}

func toListPagesJSON(resp *documentv1.ListPagesResponse) listPagesJSON {
	pages := make([]pageJSON, len(resp.GetPages()))
	for i, p := range resp.GetPages() {
		pages[i] = toPageJSON(p)
	}
	return listPagesJSON{Pages: pages, NextCursor: resp.NextCursor}
}
