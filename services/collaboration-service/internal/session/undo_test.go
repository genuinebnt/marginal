package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/flush"
	"marginal/collaboration-service/internal/oplog"
	"marginal/collaboration-service/internal/ops"
	"marginal/collaboration-service/internal/pageop"
)

func TestUndoIsNoOpWhenStackEmpty(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	results, err := s.Undo(context.Background(), uuid.Must(uuid.NewV7()), oplog.ActorUser, 0)
	require.NoError(t, err)
	assert.Empty(t, results, "undoing with nothing recorded must be a no-op, not an error")
}

func TestRedoIsNoOpWhenStackEmpty(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	results, err := s.Redo(context.Background(), uuid.Must(uuid.NewV7()), oplog.ActorUser, 0)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestUndoRevertsSingleTextOp(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	actor := uuid.Must(uuid.NewV7())
	blockID := insertBlock(t, s, actor, "hello")
	before := blockText(t, s, blockID)

	_, err := s.ApplyClientOp(context.Background(), actor, oplog.ActorUser,
		pageop.Text{BlockID: blockID, Op: ops.InsertText{At: nil, Text: " world"}}, nil, 0)
	require.NoError(t, err)
	require.NotEqual(t, before, blockText(t, s, blockID), "precondition: the insert actually changed the block")

	results, err := s.Undo(context.Background(), actor, oplog.ActorUser, 0)
	require.NoError(t, err)
	require.Len(t, results, 1, "a single-op gesture undoes as exactly one committed op")
	assert.Equal(t, before, blockText(t, s, blockID))
}

func TestUndoRevertsSingleBlockOp(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	actor := uuid.Must(uuid.NewV7())
	_ = insertBlock(t, s, actor, "hello")
	require.Len(t, s.Snapshot().Blocks, 1)

	results, err := s.Undo(context.Background(), actor, oplog.ActorUser, 0)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Empty(t, s.Snapshot().Blocks, "undoing the InsertBlock must remove the block again")
}

func TestRedoReappliesUndoneOp(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	actor := uuid.Must(uuid.NewV7())
	blockID := insertBlock(t, s, actor, "hello")

	_, err := s.Undo(context.Background(), actor, oplog.ActorUser, 0)
	require.NoError(t, err)
	assert.Empty(t, s.Snapshot().Blocks)

	results, err := s.Redo(context.Background(), actor, oplog.ActorUser, 0)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "hello", blockText(t, s, blockID))
}

func TestNewEditAfterUndoClearsRedo(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	actor := uuid.Must(uuid.NewV7())
	_ = insertBlock(t, s, actor, "hello")

	_, err := s.Undo(context.Background(), actor, oplog.ActorUser, 0)
	require.NoError(t, err)

	// A fresh edit — any edit — invalidates whatever redo was pointing at,
	// the same rule documentcore.History already enforces.
	_ = insertBlock(t, s, actor, "unrelated")

	results, err := s.Redo(context.Background(), actor, oplog.ActorUser, 0)
	require.NoError(t, err)
	assert.Empty(t, results, "redo must be gone once a new op commits for this actor")
}

// TestUndoGroupRevertsWholeGestureInOneCall is RFC-002 §3's central claim:
// "undo pops the newest undo_group belonging to this actor, never the
// newest op." Three ops sharing one undo_group must revert together, in
// one Undo call — not one twentieth of the gesture per call.
func TestUndoGroupRevertsWholeGestureInOneCall(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	actor := uuid.Must(uuid.NewV7())
	blockA := insertBlock(t, s, actor, "")

	// One gesture, two ops, sharing one undo_group: insert a second block
	// after A, then change its kind — e.g. what the "## " input rule (a
	// SetBlockKind alongside its own DeleteText) or a richer paste would
	// produce client-side. Block-tier ops address by stable BlockID, not
	// rope anchors, so this exercises the grouping/ordering logic itself
	// without also depending on doctext's own tombstone-anchor resolution.
	group := uuid.Must(uuid.NewV7())
	blockB := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	_, err := s.ApplyClientOp(context.Background(), actor, oplog.ActorUser, pageop.Block{
		Op: documentcore.InsertBlock{ID: blockB, After: &blockA, Kind: documentcore.NewParagraph(), Content: documentcore.Content{Text: "hello"}},
	}, &group, 0)
	require.NoError(t, err)
	heading, err := documentcore.NewHeading(1)
	require.NoError(t, err)
	_, err = s.ApplyClientOp(context.Background(), actor, oplog.ActorUser, pageop.Block{
		Op: documentcore.SetBlockKind{ID: blockB, From: documentcore.NewParagraph(), To: heading},
	}, &group, 0)
	require.NoError(t, err)
	require.Len(t, s.Snapshot().Blocks, 2, "precondition: both ops of the gesture landed")

	results, err := s.Undo(context.Background(), actor, oplog.ActorUser, 0)
	require.NoError(t, err)
	assert.Len(t, results, 2, "one Undo call must revert every op in the gesture")
	snap := s.Snapshot()
	require.Len(t, snap.Blocks, 1, "the block the gesture created must be gone entirely, not just its kind change")
	assert.Equal(t, blockA, snap.Blocks[0].ID)

	// And the whole gesture redoes back in one call too.
	redoResults, err := s.Redo(context.Background(), actor, oplog.ActorUser, 0)
	require.NoError(t, err)
	assert.Len(t, redoResults, 2)
	snap = s.Snapshot()
	require.Len(t, snap.Blocks, 2)
	assert.Equal(t, blockB, snap.Blocks[1].ID)
	assert.Equal(t, heading, snap.Blocks[1].Kind, "redo must restore both the reinsert and the kind change")
}

// TestUndoIsPerActorScoped is the other half of RFC-002 §3: "naive undo —
// revert the last N ops globally — is broken in a collaborative session:
// it reverts other people's work." Actor B's ops must be untouched by
// actor A's undo, even though B's op is the one most recently committed
// overall.
func TestUndoIsPerActorScoped(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	actorA := uuid.Must(uuid.NewV7())
	actorB := uuid.Must(uuid.NewV7())

	blockA := insertBlock(t, s, actorA, "from A")
	// Explicitly anchored after A's block (not the insertBlock helper's
	// default nil-After "become first") so B's insert doesn't itself shift
	// A's own recorded predecessor — that's a genuine structural conflict
	// (covered separately) this test isn't trying to exercise.
	blockB := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	_, err := s.ApplyClientOp(context.Background(), actorB, oplog.ActorUser, pageop.Block{
		Op: documentcore.InsertBlock{ID: blockB, After: &blockA, Kind: documentcore.NewParagraph(), Content: documentcore.Content{Text: "from B"}},
	}, nil, 0)
	require.NoError(t, err)

	results, err := s.Undo(context.Background(), actorA, oplog.ActorUser, 0)
	require.NoError(t, err)
	require.Len(t, results, 1)

	snap := s.Snapshot()
	require.Len(t, snap.Blocks, 1, "only A's own block must be gone")
	assert.Equal(t, blockB, snap.Blocks[0].ID)
}

// TestUndoStackSurvivesSessionReopen pins RFC-002 §3's "putting it in the
// log rather than a client-side stack": a fresh Session, opened later
// against the same durable ops, must still be able to undo an actor's
// gesture from before the reopen — an in-memory-only stack would have
// forgotten it.
func TestUndoStackSurvivesSessionReopen(t *testing.T) {
	repo := newFakeRepo()
	pageID := uuid.Must(uuid.NewV7())
	actor := uuid.Must(uuid.NewV7())
	dir := t.TempDir()

	first, err := open(context.Background(), pageID, repo, dir, "server-actor", nil,
		[]flush.Option{flush.WithBatchSize(1), flush.WithInterval(time.Hour)}, nil) // batch size 1: flushes immediately
	require.NoError(t, err)

	blockID := insertBlock(t, first, actor, "hello")
	require.Eventually(t, func() bool {
		got, _ := repo.ListForPage(context.Background(), pageID)
		return len(got) == 1
	}, time.Second, time.Millisecond, "batch size 1 must flush immediately")

	require.NoError(t, first.Close()) // orderly close before reopening the same walDir

	second, err := open(context.Background(), pageID, repo, dir, "server-actor", nil, noAutoFlush(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })
	require.Equal(t, "hello", blockText(t, second, blockID), "precondition: reopen replayed the InsertBlock")

	results, err := second.Undo(context.Background(), actor, oplog.ActorUser, 0)
	require.NoError(t, err)
	require.Len(t, results, 1, "the reopened session must still know actor's pre-reopen gesture")
	assert.Empty(t, second.Snapshot().Blocks)
}

// TestUndoConflictLeavesGroupPendingAndReportsError is the "genuinely hard
// part" RFC-002 §3 names directly: undoing an op whose precondition no
// longer holds (someone else's op landed on the same block since) must
// fail loudly — never silently corrupt state — and must leave the
// still-pending part of the gesture on the stack so a later Undo can be
// retried instead of losing it.
func TestUndoConflictLeavesGroupPendingAndReportsError(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	actor := uuid.Must(uuid.NewV7())
	other := uuid.Must(uuid.NewV7())
	blockID := insertBlock(t, s, actor, "")

	group := uuid.Must(uuid.NewV7())
	_, err := s.ApplyClientOp(context.Background(), actor, oplog.ActorUser, pageop.Block{
		Op: documentcore.SetBlockContent{Block: blockID, Prev: documentcore.Content{Text: ""}, Content: documentcore.Content{Text: "one"}},
	}, &group, 0)
	require.NoError(t, err)
	_, err = s.ApplyClientOp(context.Background(), actor, oplog.ActorUser, pageop.Block{
		Op: documentcore.SetBlockContent{Block: blockID, Prev: documentcore.Content{Text: "one"}, Content: documentcore.Content{Text: "two"}},
	}, &group, 0)
	require.NoError(t, err)

	// A different actor's own edit lands on top, unrelated to the gesture
	// above but touching the same block — this is what makes undoing the
	// gesture's last step (whose own Prev expects "two") impossible.
	_, err = s.ApplyClientOp(context.Background(), other, oplog.ActorUser, pageop.Block{
		Op: documentcore.SetBlockContent{Block: blockID, Prev: documentcore.Content{Text: "two"}, Content: documentcore.Content{Text: "conflict"}},
	}, nil, 0)
	require.NoError(t, err)

	results, err := s.Undo(context.Background(), actor, oplog.ActorUser, 0)
	require.Error(t, err, "an op whose Prev no longer matches current content must fail, not silently misapply")
	var precondition *documentcore.PreconditionError
	assert.True(t, errors.As(err, &precondition), "expected a PreconditionError, got %v", err)
	assert.Empty(t, results, "nothing in the gesture committed — the very first inverse attempted is the one that conflicts")

	snap := s.Snapshot()
	require.Len(t, snap.Blocks, 1)
	assert.Equal(t, "conflict", snap.Blocks[0].Text, "the failed undo must not have touched current content")

	// The gesture must still be there for a future retry, not silently
	// dropped by the failed attempt.
	again, err := s.Undo(context.Background(), actor, oplog.ActorUser, 0)
	require.Error(t, err)
	assert.Empty(t, again)
}
