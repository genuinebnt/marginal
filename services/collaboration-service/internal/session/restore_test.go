package session

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/flush"
	"marginal/collaboration-service/internal/oplog"
	"marginal/collaboration-service/internal/pageop"
)

// openFlushingTestSession is openTestSession's own shape, except ops
// actually reach repo (batch size 1) — RestoreTo, like Trace, only ever
// sees confirmed rows, so a test exercising it needs the flush to have
// actually happened, not just landed in the WAL.
func openFlushingTestSession(t *testing.T, repo *fakeRepo, pageID uuid.UUID) *Session {
	t.Helper()
	s, err := open(context.Background(), pageID, repo, t.TempDir(), "server-actor", nil,
		[]flush.Option{flush.WithBatchSize(1), flush.WithInterval(time.Hour)}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func requireFlushed(t *testing.T, repo *fakeRepo, pageID uuid.UUID, count int) {
	t.Helper()
	require.Eventually(t, func() bool {
		got, _ := repo.ListForPage(context.Background(), pageID)
		return len(got) == count
	}, time.Second, time.Millisecond, "op did not reach repo — batch size 1 must flush immediately")
}

// TestRestoreToRevertsMultipleStepsInOneCall pins RestoreTo's own central
// claim: it's repeated undo, not a stored snapshot swap. Three confirmed
// steps (insert A, insert B, edit A's content); restoring to right after
// step 0 must undo the last two, most-recent-first, landing on exactly
// the state that existed right after step 0 — and the whole thing must
// itself be one undo-able gesture.
func TestRestoreToRevertsMultipleStepsInOneCall(t *testing.T) {
	repo := newFakeRepo()
	pageID := uuid.Must(uuid.NewV7())
	actor := uuid.Must(uuid.NewV7())
	ctx := context.Background()
	s := openFlushingTestSession(t, repo, pageID)

	blockA := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	_, err := s.ApplyClientOp(ctx, actor, oplog.ActorUser, pageop.Block{
		Op: documentcore.InsertBlock{ID: blockA, Kind: documentcore.NewParagraph(), Content: documentcore.Content{Text: "hello"}},
	}, nil, 0)
	require.NoError(t, err)
	requireFlushed(t, repo, pageID, 1)

	blockB := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	_, err = s.ApplyClientOp(ctx, actor, oplog.ActorUser, pageop.Block{
		Op: documentcore.InsertBlock{ID: blockB, After: &blockA, Kind: documentcore.NewParagraph(), Content: documentcore.Content{Text: "world"}},
	}, nil, 0)
	require.NoError(t, err)
	requireFlushed(t, repo, pageID, 2)

	_, err = s.ApplyClientOp(ctx, actor, oplog.ActorUser, pageop.Block{
		Op: documentcore.SetBlockContent{Block: blockA, Prev: documentcore.Content{Text: "hello"}, Content: documentcore.Content{Text: "hello!"}},
	}, nil, 0)
	require.NoError(t, err)
	requireFlushed(t, repo, pageID, 3)

	snap := s.Snapshot()
	require.Len(t, snap.Blocks, 2, "precondition: both blocks and the content edit landed")

	// Restore to right after step 0 (the lone InsertBlock A) — steps 1
	// and 2 must both revert, most recent first.
	results, err := s.RestoreTo(ctx, actor, oplog.ActorUser, 0, 0)
	require.NoError(t, err)
	assert.Len(t, results, 2, "restoring past two steps must undo both in one call")

	snap = s.Snapshot()
	require.Len(t, snap.Blocks, 1, "block B must be gone — it didn't exist yet at step 0")
	assert.Equal(t, blockA, snap.Blocks[0].ID)
	assert.Equal(t, "hello", snap.Blocks[0].Text, "A's content edit must also have reverted")

	// The whole restore is itself one undo-able gesture: undoing it must
	// reapply the two reverted steps in their ORIGINAL order (insert B,
	// then edit A's content) — not the restore's own reverse order, since
	// SetBlockContent's own Prev precondition only holds against the
	// state right after InsertBlock B, not before it.
	undoResults, err := s.Undo(ctx, actor, oplog.ActorUser, 0)
	require.NoError(t, err)
	assert.Len(t, undoResults, 2, "undoing the restore must reapply both reverted steps")

	snap = s.Snapshot()
	require.Len(t, snap.Blocks, 2, "block B must be back")
	require.Equal(t, blockA, snap.Blocks[0].ID)
	assert.Equal(t, "hello!", snap.Blocks[0].Text, "A's content edit must be back too")
	assert.Equal(t, blockB, snap.Blocks[1].ID)
	assert.Equal(t, "world", snap.Blocks[1].Text)
}

func TestRestoreToCurrentStepIsNoOp(t *testing.T) {
	repo := newFakeRepo()
	pageID := uuid.Must(uuid.NewV7())
	actor := uuid.Must(uuid.NewV7())
	ctx := context.Background()
	s := openFlushingTestSession(t, repo, pageID)

	_, err := s.ApplyClientOp(ctx, actor, oplog.ActorUser, pageop.Block{
		Op: documentcore.InsertBlock{ID: documentcore.BlockID(uuid.Must(uuid.NewV7())), Kind: documentcore.NewParagraph(), Content: documentcore.Content{Text: "hello"}},
	}, nil, 0)
	require.NoError(t, err)
	requireFlushed(t, repo, pageID, 1)

	results, err := s.RestoreTo(ctx, actor, oplog.ActorUser, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, results, "restoring to the current step must be a no-op, not a redundant undo/redo round trip")
}

func TestRestoreToOutOfRangeIsRejected(t *testing.T) {
	repo := newFakeRepo()
	pageID := uuid.Must(uuid.NewV7())
	actor := uuid.Must(uuid.NewV7())
	ctx := context.Background()
	s := openFlushingTestSession(t, repo, pageID)

	_, err := s.ApplyClientOp(ctx, actor, oplog.ActorUser, pageop.Block{
		Op: documentcore.InsertBlock{ID: documentcore.BlockID(uuid.Must(uuid.NewV7())), Kind: documentcore.NewParagraph(), Content: documentcore.Content{Text: "hello"}},
	}, nil, 0)
	require.NoError(t, err)
	requireFlushed(t, repo, pageID, 1)

	_, err = s.RestoreTo(ctx, actor, oplog.ActorUser, -1, 0)
	assert.ErrorIs(t, err, ErrOutOfRange)

	_, err = s.RestoreTo(ctx, actor, oplog.ActorUser, 1, 0)
	assert.ErrorIs(t, err, ErrOutOfRange, "step 1 doesn't exist yet — only step 0 has committed")
}
