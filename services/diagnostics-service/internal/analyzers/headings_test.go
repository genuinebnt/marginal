package analyzers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/documentcore"
)

func heading(t *testing.T, level uint8) Block {
	t.Helper()
	kind, err := documentcore.NewHeading(level)
	require.NoError(t, err)
	return Block{ID: newBlockID(), Kind: kind}
}

func TestHeadingSkipFlagsH1ToH3(t *testing.T) {
	page := pageWith("a", "A", heading(t, 1), heading(t, 3))
	diags := HeadingSkips(page)
	require.Len(t, diags, 1)
	assert.Equal(t, NameHeadingSkip, diags[0].Analyzer)
	assert.Equal(t, page.Blocks[1].ID, *diags[0].BlockID)
}

func TestHeadingSkipAllowsH1ToH2ToH3(t *testing.T) {
	page := pageWith("a", "A", heading(t, 1), heading(t, 2), heading(t, 3))
	assert.Empty(t, HeadingSkips(page))
}

func TestHeadingSkipAllowsDemotingBackDown(t *testing.T) {
	// H1 -> H2 -> H1 is a normal outline shape, not a skip.
	page := pageWith("a", "A", heading(t, 1), heading(t, 2), heading(t, 1))
	assert.Empty(t, HeadingSkips(page))
}

func TestHeadingSkipIgnoresNonHeadingBlocksBetween(t *testing.T) {
	page := pageWith("a", "A", heading(t, 1), paragraph("text"), heading(t, 3))
	diags := HeadingSkips(page)
	require.Len(t, diags, 1, "a paragraph in between doesn't excuse the level jump")
}
