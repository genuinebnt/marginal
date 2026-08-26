package session

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/oplog"
	"marginal/collaboration-service/internal/pageop"
)

func TestManagerGetReturnsTheSameSessionOnRepeatedCalls(t *testing.T) {
	m := NewManager(newFakeRepo(), t.TempDir(), "server-actor", WithFlushOptions(noAutoFlush()...))
	defer func() { _ = m.CloseAll() }()
	pageID := uuid.Must(uuid.NewV7())

	s1, err := m.Get(context.Background(), pageID)
	require.NoError(t, err)
	s2, err := m.Get(context.Background(), pageID)
	require.NoError(t, err)

	assert.Same(t, s1, s2, "Get must reuse the already-open session, not reopen it")
}

func TestManagerGetOpensDistinctSessionsPerPage(t *testing.T) {
	m := NewManager(newFakeRepo(), t.TempDir(), "server-actor", WithFlushOptions(noAutoFlush()...))
	defer func() { _ = m.CloseAll() }()

	s1, err := m.Get(context.Background(), uuid.Must(uuid.NewV7()))
	require.NoError(t, err)
	s2, err := m.Get(context.Background(), uuid.Must(uuid.NewV7()))
	require.NoError(t, err)

	assert.NotSame(t, s1, s2)
}

// TestManagerCloseAllLeavesSessionsCleanlyClosable proves CloseAll
// actually closed each session's WAL writer (not just stopped its flush
// loop) — a still-open WAL file handle would make a fresh Manager's Get
// for the same page fail to open its own writer on some platforms, and
// more importantly would mean shutdown didn't really release the
// resource. Reopening against the same walDir + repo afterward, and
// finding the pre-close op still there, confirms both: the file was
// closed, and nothing was lost by closing it.
func TestManagerCloseAllLeavesSessionsCleanlyClosable(t *testing.T) {
	repo := newFakeRepo()
	dir := t.TempDir()
	pageID := uuid.Must(uuid.NewV7())
	actor := uuid.Must(uuid.NewV7())

	m1 := NewManager(repo, dir, "server-actor", WithFlushOptions(noAutoFlush()...))
	s, err := m1.Get(context.Background(), pageID)
	require.NoError(t, err)
	blockID := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	_, err = s.ApplyClientOp(context.Background(), actor, oplog.ActorUser, pageop.Block{
		Op: documentcore.InsertBlock{ID: blockID, Kind: documentcore.NewParagraph(), Content: documentcore.Content{Text: "hi"}},
	}, 0)
	require.NoError(t, err)

	require.NoError(t, m1.CloseAll())

	m2 := NewManager(repo, dir, "server-actor", WithFlushOptions(noAutoFlush()...))
	defer func() { _ = m2.CloseAll() }()
	s2, err := m2.Get(context.Background(), pageID)
	require.NoError(t, err)
	assert.Equal(t, "hi", blockText(t, s2, blockID), "the op durable at CloseAll time must survive into a fresh Manager's Open")
}
