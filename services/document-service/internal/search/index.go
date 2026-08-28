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
}

// BuildTitleIndex inserts every page's title into a fresh BK-tree —
// O(n · average-tree-depth) Levenshtein comparisons, run on Server's own
// refresh cadence, never per search request.
func BuildTitleIndex(pages []PageTitle) *TitleIndex {
	idx := &TitleIndex{tree: &bktree.Tree{}, byTitle: make(map[string][]uuid.UUID, len(pages))}
	for _, p := range pages {
		idx.tree.Insert(p.Title)
		idx.byTitle[p.Title] = append(idx.byTitle[p.Title], p.ID)
	}
	return idx
}

// Suggest fans bktree.Tree.Query's own word matches back out to every
// page currently holding that title.
func (idx *TitleIndex) Suggest(query string, maxDistance int) []Suggestion {
	var out []Suggestion
	for _, m := range idx.tree.Query(query, maxDistance) {
		for _, id := range idx.byTitle[m.Word] {
			out = append(out, Suggestion{PageID: id, Title: m.Word, Distance: m.Distance})
		}
	}
	return out
}
