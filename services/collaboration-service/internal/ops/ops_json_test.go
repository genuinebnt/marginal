package ops

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/collaboration-service/internal/anchor"
)

func TestMarshalUnmarshalOpRoundTrip(t *testing.T) {
	at := anchor.Anchor{Item: anchor.ItemID{Actor: "actor-1", Counter: 3}, Bias: anchor.Before}

	cases := []Op{
		InsertText{At: nil, Text: "hello"},
		InsertText{At: &at, Text: "XY"},
		DeleteText{
			Range: anchor.AnchorRange{
				Start: anchor.Anchor{Item: anchor.ItemID{Actor: "actor-1", Counter: 1}, Bias: anchor.Before},
				End:   anchor.Anchor{Item: anchor.ItemID{Actor: "actor-1", Counter: 4}, Bias: anchor.After},
			},
			Text: "hell",
		},
		NoOp{},
	}

	for _, op := range cases {
		data, err := MarshalOp(op)
		require.NoError(t, err)

		got, err := UnmarshalOp(data)
		require.NoError(t, err)
		assert.Equal(t, op, got)
	}
}

func TestMarshalOpUsesLowercaseWireFields(t *testing.T) {
	data, err := MarshalOp(InsertText{At: nil, Text: "hi"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"InsertText","at":null,"text":"hi"}`, string(data))
}

func TestUnmarshalOpRejectsUnknownType(t *testing.T) {
	_, err := UnmarshalOp([]byte(`{"type":"SetMark"}`))
	assert.Error(t, err)
}

type bogusOp struct{}

func (bogusOp) isOp() {}

func TestMarshalOpRejectsUnknownConcreteType(t *testing.T) {
	_, err := MarshalOp(bogusOp{})
	assert.Error(t, err)
}
