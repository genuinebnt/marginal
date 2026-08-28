package pageop

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/anchor"
	"marginal/collaboration-service/internal/ops"
)

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	blockID := documentcore.BlockID(uuid.New())
	at := anchor.Anchor{Item: anchor.ItemID{Actor: "a", Counter: 1}, Bias: anchor.After}

	tests := []struct {
		name string
		op   Op
	}{
		{
			name: "block",
			op: Block{Op: documentcore.InsertBlock{
				ID:      blockID,
				After:   nil,
				Kind:    documentcore.NewParagraph(),
				Content: documentcore.Content{Text: "hello"},
			}},
		},
		{
			// The editor's code-block language <select> writes through
			// SetBlockKind, so Language has to survive the wire like any
			// other kind field. It is the only BlockKind attribute tagged
			// omitempty (block.go), which is exactly why it needs a case:
			// "" and "go" take different marshalling paths.
			name: "block SetBlockKind carrying a code language",
			op: Block{Op: documentcore.SetBlockKind{
				ID:   blockID,
				From: documentcore.NewCodeBlock(""),
				To:   documentcore.NewCodeBlock("go"),
			}},
		},
		{
			// The reverse direction — clearing the language back to plain
			// text. omitempty drops the field entirely here, so this is the
			// case that would silently decode as "unchanged" if Unmarshal
			// ever merged onto a non-zero value instead of a fresh one.
			name: "block SetBlockKind clearing a code language",
			op: Block{Op: documentcore.SetBlockKind{
				ID:   blockID,
				From: documentcore.NewCodeBlock("go"),
				To:   documentcore.NewCodeBlock(""),
			}},
		},
		{
			name: "text",
			op:   Text{BlockID: blockID, Op: ops.InsertText{At: nil, Text: "hi"}},
		},
		{
			name: "text with anchor",
			op:   Text{BlockID: blockID, Op: ops.InsertText{At: &at, Text: "hi"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := Marshal(tc.op)
			require.NoError(t, err)

			got, err := Unmarshal(data)
			require.NoError(t, err)
			require.Equal(t, tc.op, got)
		})
	}
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
