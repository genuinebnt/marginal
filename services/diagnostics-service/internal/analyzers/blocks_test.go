package analyzers

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/documentcore"
)

func TestEmptyCodeBlockFlagsNoLanguage(t *testing.T) {
	block := Block{ID: newBlockID(), Kind: documentcore.NewCodeBlock("")}
	diags := EmptyCodeBlocks(pageWith("a", "A", block))
	require.Len(t, diags, 1)
	assert.Equal(t, NameEmptyCodeBlock, diags[0].Analyzer)
}

func TestEmptyCodeBlockIgnoresBlockWithLanguageSet(t *testing.T) {
	block := Block{ID: newBlockID(), Kind: documentcore.NewCodeBlock("go")}
	assert.Empty(t, EmptyCodeBlocks(pageWith("a", "A", block)))
}

func TestBrokenImageFlagsZeroFileID(t *testing.T) {
	block := Block{ID: newBlockID(), Kind: documentcore.NewImage(documentcore.FileID{})}
	diags := BrokenImages(pageWith("a", "A", block))
	require.Len(t, diags, 1)
	assert.Equal(t, NameBrokenImage, diags[0].Analyzer)
	assert.Equal(t, Warning, diags[0].Severity)
}

func TestBrokenImageIgnoresARealFileID(t *testing.T) {
	block := Block{ID: newBlockID(), Kind: documentcore.NewImage(documentcore.FileID(uuid.Must(uuid.NewV7())))}
	assert.Empty(t, BrokenImages(pageWith("a", "A", block)))
}
