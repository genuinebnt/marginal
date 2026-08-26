package oplog

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/ops"
	"marginal/collaboration-service/internal/pageop"
)

func TestNewStampsVersionAndUUIDv7ID(t *testing.T) {
	pageID, actorID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	l, err := New(pageID, actorID, ActorUser, nil, VectorClock{actorID.String(): 1}, testInsertTextOp())
	require.NoError(t, err)

	assert.Equal(t, Version, l.Version)
	assert.Equal(t, byte(0x7), l.ID[6]>>4, "id must be a UUIDv7 (version nibble 7)")
	assert.Equal(t, pageID, l.PageID)
}

func TestNewRejectsInvalidActorKind(t *testing.T) {
	pageID, actorID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	_, err := New(pageID, actorID, ActorKind("robot"), nil, nil, testInsertTextOp())
	assert.ErrorIs(t, err, ErrInvalidActorKind)
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	pageID, actorID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	group := uuid.Must(uuid.NewV7())
	l, err := New(pageID, actorID, ActorUser, &group, VectorClock{actorID.String(): 5}, testInsertTextOp())
	require.NoError(t, err)

	data, err := Marshal(l)
	require.NoError(t, err)

	got, err := Unmarshal(data)
	require.NoError(t, err)

	assert.Equal(t, l.ID, got.ID)
	assert.Equal(t, l.Version, got.Version)
	assert.Equal(t, l.PageID, got.PageID)
	assert.Equal(t, l.ActorID, got.ActorID)
	assert.Equal(t, l.ActorKind, got.ActorKind)
	assert.Equal(t, *l.UndoGroup, *got.UndoGroup)
	assert.Equal(t, l.VectorClock, got.VectorClock)
	assert.Equal(t, l.Op, got.Op)
	assert.True(t, l.CreatedAt.Equal(got.CreatedAt))
}

func TestUnmarshalRejectsUnsupportedVersion(t *testing.T) {
	pageID, actorID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	l, err := New(pageID, actorID, ActorUser, nil, nil, testInsertTextOp())
	require.NoError(t, err)
	l.Version = 99

	data, err := Marshal(l)
	require.NoError(t, err)

	_, err = Unmarshal(data)
	assert.ErrorIs(t, err, ErrUnsupportedVersion)
}

func testInsertTextOp() pageop.Op {
	return pageop.Text{
		BlockID: documentcore.BlockID(uuid.Must(uuid.NewV7())),
		Op:      ops.InsertText{At: nil, Text: "hello"},
	}
}

func TestLoggedOpComposesAsAStandardJSONField(t *testing.T) {
	pageID, actorID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	l, err := New(pageID, actorID, ActorUser, nil, nil, testInsertTextOp())
	require.NoError(t, err)

	type envelope struct {
		Kind string   `json:"kind"`
		Op   LoggedOp `json:"op"`
	}

	data, err := json.Marshal(envelope{Kind: "ack", Op: l})
	require.NoError(t, err)

	var got envelope
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "ack", got.Kind)
	assert.Equal(t, l.ID, got.Op.ID)
	assert.Equal(t, l.Op, got.Op.Op)
}
