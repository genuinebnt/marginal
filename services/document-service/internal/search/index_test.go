package search

import (
	"testing"

	"github.com/google/uuid"
)

// The property the v3.3.0 audit added: the BK-tree is instance-wide (one
// index, rebuilt on a cadence — one per member would be one per person),
// so the space filter has to happen at QUERY time. A suggestion IS a page
// title, so filtering it in the client would already be too late.
func TestSuggestNeverNamesATitleOutsideYourSpaces(t *testing.T) {
	mine, theirs := uuid.New(), uuid.New()
	visible, hidden := uuid.New(), uuid.New()
	idx := BuildTitleIndex([]PageTitle{
		{ID: visible, Title: "Rope internals", SpaceID: mine},
		{ID: hidden, Title: "Rope internals", SpaceID: theirs},
	})

	got := idx.Suggest("Rope internls", 2, []uuid.UUID{mine})
	if len(got) != 1 || got[0].PageID != visible {
		t.Fatalf("got %v — a title in somebody else's space was suggested", got)
	}

	// A caller with no spaces sees nothing, rather than everything: an
	// empty scope must fail closed.
	if got := idx.Suggest("Rope internls", 2, []uuid.UUID{}); len(got) != 0 {
		t.Fatalf("an empty space set returned %d suggestions", len(got))
	}
}
