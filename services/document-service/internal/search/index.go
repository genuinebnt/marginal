// Package search is SearchService's translation layer: full-text search
// via Postgres (docs.pages/docs.blocks' own GIN-indexed tsvector
// columns), plus a BK-tree title index for fuzzy "did you mean"
// matching (internal/bktree — no algorithm lives in this package beyond
// wiring bktree to real page titles and keeping it refreshed, the same
// repo.go/api.go split internal/graph already draws).
package search

import (
	"github.com/google/uuid"

	"marginal/document-service/internal/bktree"
)

// PageTitle is one live page's id/title — ListPageTitlesForIndex's own
// row, translated out of pgtype.
type PageTitle struct {
	ID    uuid.UUID
	Title string
	// Which space it belongs to. The index itself is instance-wide —
	// rebuilding one per member would be one BK-tree per person — so the
	// scoping happens when it is QUERIED, and this is what makes that
	// possible (v3.3.0; fuzzy suggestions previously named pages in
	// spaces the asker is not in).
	SpaceID uuid.UUID
}

// Suggestion is one fuzzy title match, with the real page it belongs to
// — bktree.Match only ever carries a word, since Levenshtein distance
// and the triangle inequality don't know or care what a word denotes;
// TitleIndex is what attaches a page id back onto a match.
type Suggestion struct {
	PageID   uuid.UUID
	Title    string
	Distance int
}

// TitleIndex is a snapshot of the BK-tree over every live page's title
// at the moment it was built — immutable once constructed, so Server can
// swap in a freshly-built one (on its own refresh cadence) without a
// reader ever observing a half-built tree.
type TitleIndex struct {
	tree *bktree.Tree
	// byTitle maps a title back to every page that currently has it —
	// docs.pages' own title index comment is explicit that duplicates
	// are allowed (a DuplicateTitle diagnostic, not a constraint), so
	// this is a slice, not a single id.
	byTitle map[string][]uuid.UUID
	// Which space each page is in, for the filter Suggest applies.
	spaceOf map[uuid.UUID]uuid.UUID
}

// BuildTitleIndex inserts every page's title into a fresh BK-tree —
// O(n · average-tree-depth) Levenshtein comparisons, run on Server's own
// refresh cadence, never per search request.
func BuildTitleIndex(pages []PageTitle) *TitleIndex {
	idx := &TitleIndex{
		tree: &bktree.Tree{}, byTitle: make(map[string][]uuid.UUID, len(pages)),
		spaceOf: make(map[uuid.UUID]uuid.UUID, len(pages)),
	}
	for _, p := range pages {
		idx.tree.Insert(p.Title)
		idx.byTitle[p.Title] = append(idx.byTitle[p.Title], p.ID)
		idx.spaceOf[p.ID] = p.SpaceID
	}
	return idx
}

// Suggest fans bktree.Tree.Query's own word matches back out to every
// page currently holding that title.
// visible is the caller's space set. Nil means "no filter" and is used
// only by tests of the index itself; every RPC passes a real set, and an
// EMPTY (non-nil) set yields nothing — the safe direction.
func (idx *TitleIndex) Suggest(query string, maxDistance int, visible []uuid.UUID) []Suggestion {
	var allowed map[uuid.UUID]bool
	if visible != nil {
		allowed = make(map[uuid.UUID]bool, len(visible))
		for _, id := range visible {
			allowed[id] = true
		}
	}
	var out []Suggestion
	for _, m := range idx.tree.Query(query, maxDistance) {
		for _, id := range idx.byTitle[m.Word] {
			// A title the asker may not see must not be suggested — the
			// suggestion IS the title, so filtering afterwards in the
			// client would be too late.
			if allowed != nil && !allowed[idx.spaceOf[id]] {
				continue
			}
			out = append(out, Suggestion{PageID: id, Title: m.Word, Distance: m.Distance})
		}
	}
	return out
}
