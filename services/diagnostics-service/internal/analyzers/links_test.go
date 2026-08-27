package analyzers

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/documentcore"
	"marginal/graphalgo"
)

func newBlockID() documentcore.BlockID {
	return documentcore.BlockID(uuid.Must(uuid.NewV7()))
}

func paragraph(text string) Block {
	return Block{ID: newBlockID(), Kind: documentcore.NewParagraph(), Content: documentcore.Content{Text: text}}
}

func pageWith(id, title string, blocks ...Block) Page {
	return Page{ID: id, Title: title, Blocks: blocks}
}

func TestDanglingPageLinkFlagsAnUnresolvedMention(t *testing.T) {
	page := pageWith("a", "Home", paragraph("See [[Nonexistent]] for details."))
	ctx := Context{Pages: []PageInfo{{ID: "a", Title: "Home"}}}

	diags := DanglingAndAmbiguousPageLinks(page, ctx)
	require.Len(t, diags, 1)
	assert.Equal(t, NameDanglingPageLink, diags[0].Analyzer)
	assert.Equal(t, Hint, diags[0].Severity)
	require.NotNil(t, diags[0].BlockID)
	assert.Equal(t, page.Blocks[0].ID, *diags[0].BlockID)
}

func TestPageLinkResolvingUniquelyIsNotFlagged(t *testing.T) {
	page := pageWith("a", "Home", paragraph("See [[Target]]."))
	ctx := Context{Pages: []PageInfo{{ID: "a", Title: "Home"}, {ID: "b", Title: "Target"}}}

	diags := DanglingAndAmbiguousPageLinks(page, ctx)
	assert.Empty(t, diags)
}

func TestAmbiguousPageLinkFlagsAMultiplyResolvedMention(t *testing.T) {
	page := pageWith("a", "Home", paragraph("See [[Notes]]."))
	ctx := Context{Pages: []PageInfo{
		{ID: "a", Title: "Home"}, {ID: "b", Title: "Notes"}, {ID: "c", Title: "notes"},
	}}

	diags := DanglingAndAmbiguousPageLinks(page, ctx)
	require.Len(t, diags, 1)
	assert.Equal(t, NameAmbiguousPageLink, diags[0].Analyzer)
	assert.Equal(t, Warning, diags[0].Severity)
}

func TestSelfLinkFlagsLinkingToOwnTitle(t *testing.T) {
	page := pageWith("a", "Home", paragraph("Back to [[Home]]."))
	diags := SelfLinks(page)
	require.Len(t, diags, 1)
	assert.Equal(t, NameSelfLink, diags[0].Analyzer)
	assert.Equal(t, Info, diags[0].Severity)
}

func TestSelfLinkIgnoresLinksToOtherPages(t *testing.T) {
	page := pageWith("a", "Home", paragraph("See [[Elsewhere]]."))
	assert.Empty(t, SelfLinks(page))
}

func TestLinkCycleFlagsPagesOnARealCycle(t *testing.T) {
	graph := graphalgo.Graph{
		Nodes: []graphalgo.NodeID{"a", "b", "c"},
		Edges: []graphalgo.Edge{{From: "a", To: "b"}, {From: "b", To: "c"}, {From: "c", To: "a"}},
	}
	ctx := Context{Graph: graph}

	diags := LinkCycle(pageWith("a", "A"), ctx)
	require.Len(t, diags, 1)
	assert.Equal(t, NameLinkCycle, diags[0].Analyzer)
	assert.Nil(t, diags[0].BlockID, "a cycle is a page-level fact, not one block's")
}

func TestLinkCycleIgnoresPagesNotOnTheCycle(t *testing.T) {
	graph := graphalgo.Graph{
		Nodes: []graphalgo.NodeID{"a", "b", "c", "d"},
		Edges: []graphalgo.Edge{{From: "a", To: "b"}, {From: "b", To: "a"}, {From: "c", To: "d"}},
	}
	ctx := Context{Graph: graph}

	assert.Empty(t, LinkCycle(pageWith("c", "C"), ctx))
}

func TestLinkCycleIsEmptyForAnAcyclicGraph(t *testing.T) {
	graph := graphalgo.Graph{Nodes: []graphalgo.NodeID{"a", "b"}, Edges: []graphalgo.Edge{{From: "a", To: "b"}}}
	assert.Empty(t, LinkCycle(pageWith("a", "A"), Context{Graph: graph}))
}
