package documentcore

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockKindJSONRoundTrips(t *testing.T) {
	fileID := FileID(uuid.Must(uuid.NewV7()))
	heading, err := NewHeading(2)
	require.NoError(t, err)

	tests := []struct {
		name string
		kind BlockKind
	}{
		{"paragraph", NewParagraph()},
		{"heading", heading},
		{"quote", NewQuote()},
		{"code_block", NewCodeBlock("go")},
		{"divider", NewDivider()},
		{"toggle", NewToggle()},
		{"list bulleted", NewList(Bulleted)},
		{"list numbered", NewList(Numbered)},
		{"list todo", NewList(Todo)},
		{"list_item unchecked", NewListItem(false)},
		{"list_item checked", NewListItem(true)},
		{"image", NewImage(fileID)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.kind)
			require.NoError(t, err)

			var got BlockKind
			require.NoError(t, json.Unmarshal(data, &got))
			assert.Equal(t, tc.kind, got, "json: %s", data)
		})
	}
}

func TestUnmarshalBlockKindRejectsUnknownTag(t *testing.T) {
	var k BlockKind
	err := json.Unmarshal([]byte(`{"tag":"not_a_real_kind"}`), &k)
	assert.Error(t, err)
}

func TestUnmarshalBlockKindRejectsUnknownListKind(t *testing.T) {
	var k BlockKind
	err := json.Unmarshal([]byte(`{"tag":"list","list_kind":"not_a_real_kind"}`), &k)
	var invalid *InvalidListKindError
	assert.ErrorAs(t, err, &invalid)
}

func TestUnmarshalBlockKindRejectsInvalidImageFileID(t *testing.T) {
	var k BlockKind
	err := json.Unmarshal([]byte(`{"tag":"image","file_id":"not-a-uuid"}`), &k)
	assert.Error(t, err)
}

func TestBlockTagIsContainer(t *testing.T) {
	tests := []struct {
		tag  BlockTag
		want bool
	}{
		{Paragraph, false},
		{Heading, false},
		{CodeBlock, false},
		{Divider, false},
		{Image, false},
		{Quote, true},
		{Toggle, true},
		{List, true},
		{ListItem, true},
	}
	for _, tc := range tests {
		t.Run(tc.tag.String(), func(t *testing.T) {
			assert.Equal(t, tc.want, tc.tag.IsContainer())
		})
	}
}
