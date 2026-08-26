package ops

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/collaboration-service/internal/anchor"
	"marginal/collaboration-service/internal/doctext"
)

func TestInsertTextAtStart(t *testing.T) {
	text := doctext.New("actor-1")
	inverse, err := Apply(text, InsertText{At: nil, Text: "hello"})
	require.NoError(t, err)
	assert.Equal(t, "hello", text.String())

	del, ok := inverse.(DeleteText)
	require.True(t, ok)
	assert.Equal(t, "hello", del.Text)
}

func TestInsertTextAtAnchor(t *testing.T) {
	text := doctext.New("actor-1")
	ids, err := text.InsertAt(0, "hello")
	require.NoError(t, err)

	at := anchor.Anchor{Item: ids[2], Bias: anchor.Before} // right before the first 'l'
	_, err = Apply(text, InsertText{At: &at, Text: "XY"})
	require.NoError(t, err)
	assert.Equal(t, "heXYllo", text.String())
}

func TestInsertTextEmptyStringIsNoOp(t *testing.T) {
	text := doctext.New("actor-1")
	inverse, err := Apply(text, InsertText{At: nil, Text: ""})
	require.NoError(t, err)
	assert.Equal(t, "", text.String())
	assert.IsType(t, NoOp{}, inverse)
}

func TestDeleteTextRoundTrip(t *testing.T) {
	text := doctext.New("actor-1")
	ids, err := text.InsertAt(0, "hello world")
	require.NoError(t, err)

	// delete "hello" — ids[0..5)
	del := DeleteText{
		Range: anchor.AnchorRange{
			Start: anchor.Anchor{Item: ids[0], Bias: anchor.Before},
			End:   anchor.Anchor{Item: ids[4], Bias: anchor.After},
		},
	}
	inverse, err := Apply(text, del)
	require.NoError(t, err)
	assert.Equal(t, " world", text.String())

	ins, ok := inverse.(InsertText)
	require.True(t, ok)
	assert.Equal(t, "hello", ins.Text)

	// applying the inverse restores the original text
	_, err = Apply(text, ins)
	require.NoError(t, err)
	assert.Equal(t, "hello world", text.String())
}

func TestApplyInverseIsInvolutionAcrossConcurrentEdit(t *testing.T) {
	text := doctext.New("actor-1")
	ids, err := text.InsertAt(0, "hello world")
	require.NoError(t, err)

	del := DeleteText{
		Range: anchor.AnchorRange{
			Start: anchor.Anchor{Item: ids[6], Bias: anchor.Before}, // 'w'
			End:   anchor.Anchor{Item: ids[10], Bias: anchor.After}, // 'd'
		},
	}
	inverse, err := Apply(text, del)
	require.NoError(t, err)
	assert.Equal(t, "hello ", text.String())

	// a concurrent insert lands before undo is applied
	_, err = Apply(text, InsertText{At: nil, Text: ">> "})
	require.NoError(t, err)
	assert.Equal(t, ">> hello ", text.String())

	_, err = Apply(text, inverse)
	require.NoError(t, err)
	assert.Equal(t, ">> hello world", text.String(), "undo must land next to the anchored gap, not a stale numeric offset")
}

func TestDeleteTextEmptyRangeIsNoOp(t *testing.T) {
	text := doctext.New("actor-1")
	ids, err := text.InsertAt(0, "hello")
	require.NoError(t, err)

	del := DeleteText{
		Range: anchor.AnchorRange{
			Start: anchor.Anchor{Item: ids[2], Bias: anchor.Before},
			End:   anchor.Anchor{Item: ids[2], Bias: anchor.Before},
		},
	}
	inverse, err := Apply(text, del)
	require.NoError(t, err)
	assert.Equal(t, "hello", text.String())
	assert.IsType(t, NoOp{}, inverse)
}

func TestUnknownAnchorIsRejected(t *testing.T) {
	textA := doctext.New("actor-1")
	_, err := textA.InsertAt(0, "hello")
	require.NoError(t, err)

	textB := doctext.New("actor-2")
	_, err = textB.InsertAt(0, "hello")
	require.NoError(t, err)

	foreignID := anchor.Anchor{Item: anchor.ItemID{Actor: "actor-1", Counter: 1}, Bias: anchor.Before}
	_, err = Apply(textB, InsertText{At: &foreignID, Text: "x"})
	assert.ErrorIs(t, err, ErrUnknownAnchor)
}

func TestNoOpApplyIsIdentity(t *testing.T) {
	text := doctext.New("actor-1")
	_, err := text.InsertAt(0, "hello")
	require.NoError(t, err)

	inverse, err := Apply(text, NoOp{})
	require.NoError(t, err)
	assert.Equal(t, "hello", text.String())
	assert.IsType(t, NoOp{}, inverse)
}
