package analyzers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/graphalgo"
)

func TestDuplicateTitleFlagsASharedTitle(t *testing.T) {
	ctx := Context{Pages: []PageInfo{{ID: "a", Title: "Notes"}, {ID: "b", Title: "notes"}}}
	diags := DuplicateTitle(pageWith("a", "Notes"), ctx)
	require.Len(t, diags, 1)
	assert.Equal(t, NameDuplicateTitle, diags[0].Analyzer)
	assert.Equal(t, Warning, diags[0].Severity)
}

func TestDuplicateTitleIgnoresAUniqueTitle(t *testing.T) {
	ctx := Context{Pages: []PageInfo{{ID: "a", Title: "Notes"}, {ID: "b", Title: "Other"}}}
	assert.Empty(t, DuplicateTitle(pageWith("a", "Notes"), ctx))
}

// TestOrphanPageFlagsAMutuallyLinkedPairNotJustBacklinksZero pins the
// exact argument graphalgo.Orphans' own doc comment makes: a and b link
// to each other (each has a nonzero backlink), but neither is a root, so
// both are still orphaned.
func TestOrphanPageFlagsAMutuallyLinkedPairNotJustBacklinksZero(t *testing.T) {
	ctx := Context{
		Pages: []PageInfo{{ID: "root", Title: "Root", IsRoot: true}, {ID: "a", Title: "A"}, {ID: "b", Title: "B"}},
		Graph: graphalgo.Graph{
			Nodes: []graphalgo.NodeID{"root", "a", "b"},
			Edges: []graphalgo.Edge{{From: "a", To: "b"}, {From: "b", To: "a"}},
		},
	}
	diags := OrphanPage(pageWith("a", "A"), ctx)
	require.Len(t, diags, 1)
	assert.Equal(t, NameOrphanPage, diags[0].Analyzer)
	assert.Equal(t, Info, diags[0].Severity)
}

func TestOrphanPageIgnoresARootPage(t *testing.T) {
	ctx := Context{
		Pages: []PageInfo{{ID: "root", Title: "Root", IsRoot: true}},
		Graph: graphalgo.Graph{Nodes: []graphalgo.NodeID{"root"}},
	}
	assert.Empty(t, OrphanPage(pageWith("root", "Root"), ctx))
}

func TestOrphanPageIgnoresAPageReachableFromARoot(t *testing.T) {
	ctx := Context{
		Pages: []PageInfo{{ID: "root", Title: "Root", IsRoot: true}, {ID: "a", Title: "A"}},
		Graph: graphalgo.Graph{
			Nodes: []graphalgo.NodeID{"root", "a"},
			Edges: []graphalgo.Edge{{From: "root", To: "a"}},
		},
	}
	assert.Empty(t, OrphanPage(pageWith("a", "A"), ctx))
}
