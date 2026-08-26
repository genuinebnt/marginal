package anchor

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBiasMarshalsAsAReadableString(t *testing.T) {
	data, err := json.Marshal(Before)
	require.NoError(t, err)
	assert.Equal(t, `"before"`, string(data))

	data, err = json.Marshal(After)
	require.NoError(t, err)
	assert.Equal(t, `"after"`, string(data))
}

func TestBiasRoundTripsThroughJSON(t *testing.T) {
	for _, b := range []Bias{Before, After} {
		data, err := json.Marshal(b)
		require.NoError(t, err)
		var got Bias
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, b, got)
	}
}

func TestBiasUnmarshalRejectsUnknownString(t *testing.T) {
	var b Bias
	err := json.Unmarshal([]byte(`"sideways"`), &b)
	assert.Error(t, err)
}

func TestAnchorAndAnchorRangeUseLowercaseWireFields(t *testing.T) {
	// Pins the wire contract collaboration's WebSocket layer depends on —
	// matching documentcore's own lowercase json tag convention
	// (services/document-service/internal/documentcore/operation.go), not
	// Go's default capitalized field names.
	r := AnchorRange{
		Start: Anchor{Item: ItemID{Actor: "a", Counter: 1}, Bias: Before},
		End:   Anchor{Item: ItemID{Actor: "a", Counter: 2}, Bias: After},
	}
	data, err := json.Marshal(r)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"start": {"item": {"actor": "a", "counter": 1}, "bias": "before"},
		"end":   {"item": {"actor": "a", "counter": 2}, "bias": "after"}
	}`, string(data))
}
