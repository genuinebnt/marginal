package pageop

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/anchor"
	"marginal/collaboration-service/internal/ops"
)

func TestMarshalUnmarshalRoundTripBlock(t *testing.T) {
	blockID := documentcore.BlockID(uuid.New())
	op := Block{Op: documentcore.InsertBlock{
		ID:      blockID,
		After:   nil,
		Kind:    documentcore.NewParagraph(),
		Content: documentcore.Content{Text: "hello"},
	}}

	data, err := Marshal(op)
	require.NoError(t, err)

	got, err := Unmarshal(data)
	require.NoError(t, err)
	require.Equal(t, op, got)
}

func TestMarshalUnmarshalRoundTripText(t *testing.T) {
	blockID := documentcore.BlockID(uuid.New())
	op := Text{
		BlockID: blockID,
		Op:      ops.InsertText{At: nil, Text: "hi"},
	}

	data, err := Marshal(op)
	require.NoError(t, err)

	got, err := Unmarshal(data)
	require.NoError(t, err)
	require.Equal(t, op, got)
}

func TestMarshalUnmarshalRoundTripTextWithAnchor(t *testing.T) {
	blockID := documentcore.BlockID(uuid.New())
	at := anchor.Anchor{Item: anchor.ItemID{Actor: "a", Counter: 1}, Bias: anchor.After}
	op := Text{
		BlockID: blockID,
		Op:      ops.InsertText{At: &at, Text: "hi"},
	}

	data, err := Marshal(op)
	require.NoError(t, err)

	got, err := Unmarshal(data)
	require.NoError(t, err)
	require.Equal(t, op, got)
}

func TestTypeNamePrefixesByTier(t *testing.T) {
	blockName, err := TypeName(Block{Op: documentcore.SetTitle{}})
	require.NoError(t, err)
	require.Equal(t, "block:SetTitle", blockName)

	textName, err := TypeName(Text{Op: ops.NoOp{}})
	require.NoError(t, err)
	require.Equal(t, "text:NoOp", textName)
}

func TestUnmarshalUnknownScopeErrors(t *testing.T) {
	_, err := Unmarshal([]byte(`{"scope":"nonsense"}`))
	require.Error(t, err)
}
