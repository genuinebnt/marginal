package doctext

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/collaboration-service/internal/anchor"
)

func TestInsertAndString(t *testing.T) {
	text := New("actor-1")
	_, err := text.InsertAt(0, "hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", text.String())
	assert.Equal(t, 5, text.RuneLen())
}

func TestInsertAtMiddle(t *testing.T) {
	text := New("actor-1")
	_, err := text.InsertAt(0, "hello")
	require.NoError(t, err)
	_, err = text.InsertAt(2, "XY")
	require.NoError(t, err)
	assert.Equal(t, "heXYllo", text.String())
}

func TestDeleteRange(t *testing.T) {
	text := New("actor-1")
	_, err := text.InsertAt(0, "hello world")
	require.NoError(t, err)
	require.NoError(t, text.DeleteRange(5, 11))
	assert.Equal(t, "hello", text.String())
}

func TestMultibyteRunesCountAsOneOffsetEach(t *testing.T) {
	text := New("actor-1")
	_, err := text.InsertAt(0, "héllo") // 'é' is 2 bytes, 1 rune
	require.NoError(t, err)
	assert.Equal(t, 5, text.RuneLen(), "rune length, not byte length")

	_, err = text.InsertAt(2, "X") // right after 'é', a rune boundary — must not need byte-boundary gymnastics
	require.NoError(t, err)
	assert.Equal(t, "héXllo", text.String())
}

func TestResolveTracksAnAnchorAcrossEdits(t *testing.T) {
	text := New("actor-1")
	ids, err := text.InsertAt(0, "hello")
	require.NoError(t, err)

	_, err = text.InsertAt(0, "XY") // shifts everything right by 2
	require.NoError(t, err)

	got := text.Resolve(anchor.Anchor{Item: ids[2], Bias: anchor.Before}) // originally the 'l' at rune 2
	assert.Equal(t, anchor.Resolved{Kind: anchor.At, Offset: 4}, got)
	assert.Equal(t, "XYhello", text.String())
	assert.Equal(t, byte('l'), text.String()[got.Offset])
}

// TestConcurrentEditEndToEnd is the scenario the whole package exists
// for: client B composes an insert anchored at "right before ids[2]"
// against the document as it looked before client A's concurrent edit
// landed. Applying B's op after A's must still land the new text next to
// the character B actually meant, not at the stale numeric offset 2.
func TestConcurrentEditEndToEnd(t *testing.T) {
	text := New("actor-1")
	ids, err := text.InsertAt(0, "hello")
	require.NoError(t, err)

	bAnchor := anchor.Anchor{Item: ids[2], Bias: anchor.Before} // "right before the second 'l'... " wait, ids[2] is the first 'l'

	// Client A's concurrent insert of "XY" at the start lands first.
	_, err = text.InsertAt(0, "XY")
	require.NoError(t, err)
	require.Equal(t, "XYhello", text.String())

	// Client B's op resolves its anchor against the CURRENT document, not
	// the offset (2) it was originally composed against.
	resolved := text.Resolve(bAnchor)
	require.Equal(t, anchor.At, resolved.Kind)
	_, err = text.InsertAt(resolved.Offset, "-")
	require.NoError(t, err)
	assert.Equal(t, "XYhe-llo", text.String(), "B's insert must land next to the character it anchored to, not at its stale offset")
}

func TestDeleteRejectsOutOfBoundsAndInvertedRange(t *testing.T) {
	text := New("actor-1")
	_, err := text.InsertAt(0, "hello")
	require.NoError(t, err)

	assert.ErrorIs(t, text.DeleteRange(3, 1), ErrInvertedRange)
	assert.ErrorIs(t, text.DeleteRange(0, 99), ErrOutOfBounds)
}

func TestResolveDetachedAfterDeletion(t *testing.T) {
	text := New("actor-1")
	ids, err := text.InsertAt(0, "hello")
	require.NoError(t, err)

	require.NoError(t, text.DeleteRange(1, 4)) // delete "ell", leaving "ho"

	got := text.Resolve(anchor.Anchor{Item: ids[2], Bias: anchor.Before})
	assert.Equal(t, anchor.Detached, got.Kind)
	assert.Equal(t, 1, got.Offset)
}
