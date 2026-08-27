package session

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/flush"
	"marginal/collaboration-service/internal/oplog"
	"marginal/collaboration-service/internal/ops"
	"marginal/collaboration-service/internal/pageop"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// noAutoFlush makes flush.Loop effectively never fire on its own during a
// test — assertions read straight from the Session/fakeRepo, not from
// racing against a background flush.
func noAutoFlush() []flush.Option {
	return []flush.Option{flush.WithBatchSize(1_000_000), flush.WithInterval(time.Hour)}
}

func openTestSession(t *testing.T, repo *fakeRepo, pageID uuid.UUID) *Session {
	t.Helper()
	s, err := open(context.Background(), pageID, repo, t.TempDir(), "server-actor", nil, noAutoFlush(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// insertBlock submits an InsertBlock op with initial text and returns its
// id — every test below needs a block to exist before it can send a Text
// op against it, since a Session no longer has one whole-page rope.
func insertBlock(t *testing.T, s *Session, actor uuid.UUID, text string) documentcore.BlockID {
	t.Helper()
	id := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	_, err := s.ApplyClientOp(context.Background(), actor, oplog.ActorUser, pageop.Block{
		Op: documentcore.InsertBlock{ID: id, Kind: documentcore.NewParagraph(), Content: documentcore.Content{Text: text}},
	}, nil, 0)
	require.NoError(t, err)
	return id
}

func blockText(t *testing.T, s *Session, id documentcore.BlockID) string {
	t.Helper()
	for _, b := range s.Snapshot().Blocks {
		if b.ID == id {
			return b.Text
		}
	}
	t.Fatalf("block %s not found in snapshot", id)
	return ""
}

func TestOpenWithNoHistoryIsEmpty(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	assert.Empty(t, s.Snapshot().Blocks)
}

func TestOpenReplaysConfirmedOpsInOrder(t *testing.T) {
	repo := newFakeRepo()
	pageID := uuid.Must(uuid.NewV7())
	actor := uuid.Must(uuid.NewV7())
	blockID := documentcore.BlockID(uuid.Must(uuid.NewV7()))

	l1, err := oplog.New(pageID, actor, oplog.ActorUser, nil, nil, pageop.Block{
		Op: documentcore.InsertBlock{ID: blockID, Kind: documentcore.NewParagraph(), Content: documentcore.Content{Text: "hello"}},
	})
	require.NoError(t, err)
	_, err = repo.Append(context.Background(), l1)
	require.NoError(t, err)

	s := openTestSession(t, repo, pageID)
	assert.Equal(t, "hello", blockText(t, s, blockID))
}

// TestSnapshotIncludesMarksSetViaSetBlockContent pins a real gap that
// existed before BlockSnapshot grew a Marks field: documentcore.Page.Apply
// already stores a block's whole Content (text and marks together) when a
// SetBlockContent op lands, but nothing surfaced Marks to a client — a
// mark applied, then a reconnect, would have silently lost it even though
// the session's own in-memory Page never did.
func TestSnapshotIncludesMarksSetViaSetBlockContent(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	actor := uuid.Must(uuid.NewV7())
	blockID := insertBlock(t, s, actor, "hello world")

	bold := documentcore.Mark{Kind: documentcore.NewBold(), Start: 0, End: 5}
	_, err := s.ApplyClientOp(context.Background(), actor, oplog.ActorUser, pageop.Block{
		Op: documentcore.SetBlockContent{
			Block:   blockID,
			Prev:    documentcore.Content{Text: "hello world"},
			Content: documentcore.Content{Text: "hello world", Marks: []documentcore.Mark{bold}},
		},
	}, nil, 0)
	require.NoError(t, err)

	snap := s.Snapshot()
	require.Len(t, snap.Blocks, 1)
	require.Len(t, snap.Blocks[0].Marks, 1, "a mark set via SetBlockContent must appear in a fresh snapshot, not just in the live in-memory Page")
	assert.Equal(t, bold, snap.Blocks[0].Marks[0])
}

func TestApplyClientOpMutatesTextAndReturnsAck(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	actor := uuid.Must(uuid.NewV7())
	blockID := insertBlock(t, s, actor, "")

	textOp := pageop.Text{BlockID: blockID, Op: ops.InsertText{At: nil, Text: "hi"}}
	result, err := s.ApplyClientOp(context.Background(), actor, oplog.ActorUser, textOp, nil, 0)
	require.NoError(t, err)
	assert.Equal(t, "hi", blockText(t, s, blockID))
	assert.Equal(t, actor, result.Op.ActorID)
	assert.Equal(t, oplog.ActorUser, result.Op.ActorKind)
	assert.Equal(t, textOp, result.Op.Op)
	require.NotNil(t, result.Boundaries, "a non-empty block must report its boundary anchors")
}

func TestBroadcastExcludesTheSubmittingSubscriber(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	actor := uuid.Must(uuid.NewV7())
	blockID := insertBlock(t, s, actor, "")

	subARec := &recordingSubscriber{}
	subA := s.Subscribe(uuid.Must(uuid.NewV7()), subARec)
	defer subA.Close()

	subBRec := &recordingSubscriber{}
	subB := s.Subscribe(uuid.Must(uuid.NewV7()), subBRec)
	defer subB.Close()

	result, err := s.ApplyClientOp(context.Background(), actor, oplog.ActorUser,
		pageop.Text{BlockID: blockID, Op: ops.InsertText{At: nil, Text: "hi"}}, nil, subA.ID)
	require.NoError(t, err)

	assert.Empty(t, subARec.snapshot(), "the submitting subscriber must not also receive its own op via Deliver")
	got := subBRec.snapshot()
	require.Len(t, got, 1)
	assert.Equal(t, result.Op.ID, got[0].ID)
}

func TestUnsubscribeStopsFurtherDelivery(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	actor := uuid.Must(uuid.NewV7())
	blockID := insertBlock(t, s, actor, "")

	rec := &recordingSubscriber{}
	sub := s.Subscribe(uuid.Must(uuid.NewV7()), rec)
	sub.Close()

	_, err := s.ApplyClientOp(context.Background(), actor, oplog.ActorUser,
		pageop.Text{BlockID: blockID, Op: ops.InsertText{At: nil, Text: "hi"}}, nil, 999)
	require.NoError(t, err)
	assert.Empty(t, rec.snapshot())
}

func TestCanApplyDeniesWithoutMutatingState(t *testing.T) {
	repo := newFakeRepo()
	pageID := uuid.Must(uuid.NewV7())
	deny := func(pageop.Op, uuid.UUID, oplog.ActorKind) bool { return false }

	s, err := open(context.Background(), pageID, repo, t.TempDir(), "server-actor", deny, noAutoFlush(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })

	blockID := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	_, err = s.ApplyClientOp(context.Background(), uuid.Must(uuid.NewV7()), oplog.ActorUser,
		pageop.Block{Op: documentcore.InsertBlock{ID: blockID, Kind: documentcore.NewParagraph()}}, nil, 0)
	assert.ErrorIs(t, err, ErrDenied)
	assert.Empty(t, s.Snapshot().Blocks, "a denied op must not touch the page")
}

// TestOpenRecoversUnflushedWALOps is the scenario internal/wal and this
// package's own doc comment exist for: an orderly-enough shutdown (or a
// crash) leaves ops durably WAL-synced but never reached Postgres because
// the flush loop hadn't fired yet. The next Open for that page must
// reconstruct the exact same page/block state and re-drive those ops into
// the repo — not silently lose them, and not double-apply the ones that
// *did* make it.
func TestOpenRecoversUnflushedWALOps(t *testing.T) {
	repo := newFakeRepo()
	pageID := uuid.Must(uuid.NewV7())
	actor := uuid.Must(uuid.NewV7())
	dir := t.TempDir()

	first, err := open(context.Background(), pageID, repo, dir, "server-actor", nil, noAutoFlush(), nil)
	require.NoError(t, err)

	blockID := insertBlock(t, first, actor, "")
	_, err = first.ApplyClientOp(context.Background(), actor, oplog.ActorUser,
		pageop.Text{BlockID: blockID, Op: ops.InsertText{At: nil, Text: "hello "}}, nil, 0)
	require.NoError(t, err)
	_, err = first.ApplyClientOp(context.Background(), actor, oplog.ActorUser,
		pageop.Text{BlockID: blockID, Op: ops.InsertText{At: nil, Text: "world"}}, nil, 0)
	require.NoError(t, err)
	require.Equal(t, "worldhello ", blockText(t, first, blockID), "InsertAt(nil) always inserts at position 0")

	// Simulate a crash: the WAL file handle is abandoned directly,
	// bypassing flush.Loop.Stop's own drain-and-flush — repo must still
	// be empty for this page (noAutoFlush), proving none of the three ops
	// above (InsertBlock plus the two text ops) ever reached it.
	require.NoError(t, first.wal.Close())
	got, err := repo.ListForPage(context.Background(), pageID)
	require.NoError(t, err)
	require.Empty(t, got, "precondition: nothing flushed to the repo yet")

	second, err := open(context.Background(), pageID, repo, dir, "server-actor", nil, noAutoFlush(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })
	t.Cleanup(func() { first.flush.Stop() }) // appease goleak; assertions are already done

	assert.Equal(t, blockText(t, first, blockID), blockText(t, second, blockID), "recovery must reconstruct the exact pre-crash block state")

	recovered, err := repo.ListForPage(context.Background(), pageID)
	require.NoError(t, err)
	assert.Len(t, recovered, 3, "the InsertBlock plus both un-flushed text ops must all be reflushed by Open's reconciliation")
}

func TestOpenDoesNotDoubleApplyAlreadyConfirmedOps(t *testing.T) {
	repo := newFakeRepo()
	pageID := uuid.Must(uuid.NewV7())
	actor := uuid.Must(uuid.NewV7())
	dir := t.TempDir()

	first, err := open(context.Background(), pageID, repo, dir, "server-actor", nil,
		[]flush.Option{flush.WithBatchSize(1), flush.WithInterval(time.Hour)}, nil) // batch size 1: flushes immediately
	require.NoError(t, err)

	blockID := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	_, err = first.ApplyClientOp(context.Background(), actor, oplog.ActorUser,
		pageop.Block{Op: documentcore.InsertBlock{ID: blockID, Kind: documentcore.NewParagraph(), Content: documentcore.Content{Text: "hi"}}}, nil, 0)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		got, _ := repo.ListForPage(context.Background(), pageID)
		return len(got) == 1
	}, time.Second, time.Millisecond, "batch size 1 must flush immediately")

	require.NoError(t, first.Close()) // orderly close: flush loop drains (nothing left to drain)

	second, err := open(context.Background(), pageID, repo, dir, "server-actor", nil, noAutoFlush(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })

	assert.Equal(t, "hi", blockText(t, second, blockID), "must not be duplicated — the WAL record for the already-confirmed op must be skipped, not reapplied")

	got, err := repo.ListForPage(context.Background(), pageID)
	require.NoError(t, err)
	assert.Len(t, got, 1, "reconciliation must not create a duplicate row for an already-confirmed op")
}

func TestSubscribeReportsAlreadyPresentActorsAndBroadcastsJoinLeave(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	actorA := uuid.Must(uuid.NewV7())
	actorB := uuid.Must(uuid.NewV7())

	recA := &recordingSubscriber{}
	subA := s.Subscribe(actorA, recA)
	defer subA.Close()
	assert.Empty(t, subA.Present, "the first subscriber has nobody already present")

	recB := &recordingSubscriber{}
	subB := s.Subscribe(actorB, recB)
	require.Len(t, subB.Present, 1, "B's own join must report A, who was already here")
	assert.Equal(t, actorA, subB.Present[0])

	joinEvents := recA.presenceSnapshot()
	require.Len(t, joinEvents, 1, "A must be told about B's join")
	assert.Equal(t, actorB, joinEvents[0].ActorID)
	assert.True(t, joinEvents[0].Joined)
	assert.Empty(t, recB.presenceSnapshot(), "B must not be told about its own join")

	subB.Close()
	leaveEvents := recA.presenceSnapshot()
	require.Len(t, leaveEvents, 2)
	assert.Equal(t, actorB, leaveEvents[1].ActorID)
	assert.False(t, leaveEvents[1].Joined)
}

func TestSubscribeIgnoresASecondConnectionFromTheSameActor(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	actorA := uuid.Must(uuid.NewV7())
	observer := uuid.Must(uuid.NewV7())

	obsRec := &recordingSubscriber{}
	subObs := s.Subscribe(observer, obsRec)
	defer subObs.Close()

	tab1Rec := &recordingSubscriber{}
	subTab1 := s.Subscribe(actorA, tab1Rec)
	require.Len(t, obsRec.presenceSnapshot(), 1, "the observer sees A's first connection join")

	tab2Rec := &recordingSubscriber{}
	subTab2 := s.Subscribe(actorA, tab2Rec)
	assert.Len(t, obsRec.presenceSnapshot(), 1, "a second connection from an already-present actor must not re-announce a join")

	subTab1.Close()
	assert.Len(t, obsRec.presenceSnapshot(), 1, "actor A is still present via tab2 — no leave yet")

	subTab2.Close()
	events := obsRec.presenceSnapshot()
	require.Len(t, events, 2, "only the LAST connection closing announces a leave")
	assert.False(t, events[1].Joined)
}

func TestSetCursorBroadcastsAndSeedsLaterJoinersSnapshot(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	actorA := uuid.Must(uuid.NewV7())
	actorB := uuid.Must(uuid.NewV7())
	blockID := documentcore.BlockID(uuid.Must(uuid.NewV7()))

	recA := &recordingSubscriber{}
	subA := s.Subscribe(actorA, recA)
	defer subA.Close()

	recB := &recordingSubscriber{}
	subB := s.Subscribe(actorB, recB)
	defer subB.Close()

	s.SetCursor(actorA, CursorEvent{BlockID: &blockID, Start: 3, End: 7}, subA.ID)

	bEvents := recB.cursorSnapshot()
	require.Len(t, bEvents, 1, "B must be told about A's cursor move")
	assert.Equal(t, actorA, bEvents[0].ActorID)
	require.NotNil(t, bEvents[0].BlockID)
	assert.Equal(t, blockID, *bEvents[0].BlockID)
	assert.Equal(t, 3, bEvents[0].Start)
	assert.Equal(t, 7, bEvents[0].End)
	assert.Empty(t, recA.cursorSnapshot(), "A must not be told about its own cursor move")

	// A later joiner's Subscribe must be seeded with A's still-current cursor.
	recC := &recordingSubscriber{}
	subC := s.Subscribe(uuid.Must(uuid.NewV7()), recC)
	defer subC.Close()
	require.Len(t, subC.Cursors, 1, "C's own join must be seeded with A's last-known cursor")
	assert.Equal(t, actorA, subC.Cursors[0].ActorID)

	// Clearing the cursor (blurring out of every block) removes it from
	// the next joiner's seed and is itself broadcast.
	s.SetCursor(actorA, CursorEvent{BlockID: nil}, subA.ID)
	clearEvents := recB.cursorSnapshot()
	require.Len(t, clearEvents, 2)
	assert.Nil(t, clearEvents[1].BlockID)

	recD := &recordingSubscriber{}
	subD := s.Subscribe(uuid.Must(uuid.NewV7()), recD)
	defer subD.Close()
	assert.Empty(t, subD.Cursors, "a cleared cursor must not linger for a later joiner")
}

func TestUnsubscribeClearsTheLeavingActorsCursor(t *testing.T) {
	s := openTestSession(t, newFakeRepo(), uuid.Must(uuid.NewV7()))
	actorA := uuid.Must(uuid.NewV7())
	blockID := documentcore.BlockID(uuid.Must(uuid.NewV7()))

	recA := &recordingSubscriber{}
	subA := s.Subscribe(actorA, recA)
	s.SetCursor(actorA, CursorEvent{BlockID: &blockID, Start: 0, End: 0}, subA.ID)

	recB := &recordingSubscriber{}
	subB := s.Subscribe(uuid.Must(uuid.NewV7()), recB)
	defer subB.Close()
	require.Len(t, subB.Cursors, 1, "B joins while A's cursor is still live")

	subA.Close()
	leaveClears := recB.cursorSnapshot()
	require.NotEmpty(t, leaveClears, "A leaving must clear their cursor for whoever's still here")
	last := leaveClears[len(leaveClears)-1]
	assert.Equal(t, actorA, last.ActorID)
	assert.Nil(t, last.BlockID)

	recC := &recordingSubscriber{}
	subC := s.Subscribe(uuid.Must(uuid.NewV7()), recC)
	defer subC.Close()
	assert.Empty(t, subC.Cursors, "a departed actor's stale cursor must not linger for a later joiner")
}
