package documentcore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMarshalOpRoundTrips locks in the wire format MarshalOp/UnmarshalOp
// use — the same format the WASM/JS boundary (cmd/wasm) and, later,
// DATA_MODEL.md's collab.ops payload column both depend on.
func TestMarshalOpRoundTrips(t *testing.T) {
	for _, op := range exampleOps() {
		data, err := MarshalOp(op)
		require.NoError(t, err, "marshal %#v", op)

		got, err := UnmarshalOp(data)
		require.NoError(t, err, "unmarshal %s", data)

		assert.True(t, opsEqual(op, got), "round trip changed the op:\nwant: %#v\ngot:  %#v\njson: %s", op, got, data)
	}
}

func TestMarshalOpIncludesTypeField(t *testing.T) {
	data, err := MarshalOp(SetTitle{Page: newTestPageID(), From: "a", To: "b"})
	require.NoError(t, err)
	assert.Contains(t, string(data), `"type":"SetTitle"`)
}

func TestUnmarshalOpRejectsUnknownType(t *testing.T) {
	_, err := UnmarshalOp([]byte(`{"type":"NotARealOp"}`))
	assert.Error(t, err)
}
