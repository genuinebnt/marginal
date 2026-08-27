package session

import (
	"context"
	"encoding/json"
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

// openFastFlushSession is openTestSession but with an immediate-flush
// loop (batch size 1) instead of noAutoFlush's — Trace reads confirmed
// ops from the repo, not a session's own in-memory/WAL state, so these
// tests need what they commit to actually reach it.
func openFastFlushSession(t *testing.T, repo *fakeRepo, pageID uuid.UUID) *Session {
	t.Helper()
	s, err := open(context.Background(), pageID, repo, t.TempDir(), "server-actor", nil,
		[]flush.Option{flush.WithBatchSize(1), flush.WithInterval(time.Hour)}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func waitForFlushedCount(t *testing.T, repo *fakeRepo, pageID uuid.UUID, n int) {
	t.Helper()
	require.Eventually(t, func() bool {
		got, _ := repo.ListForPage(context.Background(), pageID)
		return len(got) == n
	}, time.Second, time.Millisecond, "expected %d ops to have flushed", n)
}

func TestTraceOnEmptyPageIsEmpty(t *testing.T) {
	steps, err := Trace(context.Background(), uuid.Must(uuid.NewV7()), newFakeRepo(), "server-actor")
	require.NoError(t, err)
	assert.Empty(t, steps)
}

// TestTraceLawHoldsForARealSequence pins trace.html's own central claim —
// "apply(invert(op), apply(op, doc)) == doc" is CHECKED, not asserted —
// against a real, mixed block+text sequence committed through the actual
// live pipeline (not hand-built LoggedOps), then read back via Trace.
func TestTraceLawHoldsForARealSequence(t *testing.T) {
	repo := newFakeRepo()
	pageID := uuid.Must(uuid.NewV7())
	actor := uuid.Must(uuid.NewV7())
	s := openFastFlushSession(t, repo, pageID)

	blockID := insertBlock(t, s, actor, "")
	_, err := s.ApplyClientOp(context.Background(), actor, oplog.ActorUser,
		pageop.Text{BlockID: blockID, Op: ops.InsertText{At: nil, Text: "hello"}}, nil, 0)
	require.NoError(t, err)
	heading, err := documentcore.NewHeading(2)
	require.NoError(t, err)
	_, err = s.ApplyClientOp(context.Background(), actor, oplog.ActorUser, pageop.Block{
		Op: documentcore.SetBlockKind{ID: blockID, From: documentcore.NewParagraph(), To: heading},
	}, nil, 0)
	require.NoError(t, err)
	waitForFlushedCount(t, repo, pageID, 3)

	steps, err := Trace(context.Background(), pageID, repo, "server-actor")
	require.NoError(t, err)
	require.Len(t, steps, 3, "InsertBlock, InsertText, SetBlockKind")
	for i, step := range steps {
		assert.True(t, step.LawHolds, "step %d (%s) must satisfy the invertibility law", i, step.Op.ID)
	}

	require.Len(t, steps[0].After.Blocks, 1, "After InsertBlock: the block exists")
	assert.Equal(t, "", steps[0].After.Blocks[0].Text)
	require.Len(t, steps[1].After.Blocks, 1, "After InsertText: still one block")
	assert.Equal(t, "hello", steps[1].After.Blocks[0].Text)
	require.Len(t, steps[2].After.Blocks, 1, "After SetBlockKind: kind changed, text untouched")
	assert.Equal(t, "hello", steps[2].After.Blocks[0].Text)
	assert.Equal(t, heading, steps[2].After.Blocks[0].Kind)
}

// TestTraceOutputRoundTripsThroughTheWireEnvelope pins the exact reason
// TraceStep has a custom MarshalJSON: encoding/json's default handling of
// an interface field silently drops the "type" tag pageop.Op needs to be
// decodable at all — the same gap oplog.LoggedOp's own doc comment
// already names for the ack/broadcast frames.
func TestTraceOutputRoundTripsThroughTheWireEnvelope(t *testing.T) {
	repo := newFakeRepo()
	pageID := uuid.Must(uuid.NewV7())
	actor := uuid.Must(uuid.NewV7())
	s := openFastFlushSession(t, repo, pageID)
	_ = insertBlock(t, s, actor, "hi")
	waitForFlushedCount(t, repo, pageID, 1)

	steps, err := Trace(context.Background(), pageID, repo, "server-actor")
	require.NoError(t, err)
	require.Len(t, steps, 1)

	raw, err := steps[0].MarshalJSON()
	require.NoError(t, err)

	var decoded struct {
		Op struct {
			Op json.RawMessage `json:"op"`
		} `json:"op"`
		Inverse  json.RawMessage `json:"inverse"`
		LawHolds bool            `json:"law_holds"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.True(t, decoded.LawHolds)

	decodedOp, err := pageop.Unmarshal(decoded.Op.Op)
	require.NoError(t, err, "the op field must still decode back into a real pageop.Op")
	block, ok := decodedOp.(pageop.Block)
	require.True(t, ok)
	_, ok = block.Op.(documentcore.InsertBlock)
	assert.True(t, ok)

	decodedInverse, err := pageop.Unmarshal(decoded.Inverse)
	require.NoError(t, err, "the inverse field must decode too")
	invBlock, ok := decodedInverse.(pageop.Block)
	require.True(t, ok)
	_, ok = invBlock.Op.(documentcore.DeleteBlock)
	assert.True(t, ok, "InsertBlock's inverse must be DeleteBlock")
}

// TestTraceReturnsErrorWhenReplayFails is the honest-failure case: a
// corrupt or malformed log entry (here, a "text" op naming a block that
// was never inserted — the same ErrUnknownBlock a live session would hit)
// must surface as an error from Trace, never a silently-wrong LawHolds.
func TestTraceReturnsErrorWhenReplayFails(t *testing.T) {
	repo := newFakeRepo()
	pageID := uuid.Must(uuid.NewV7())
	actor := uuid.Must(uuid.NewV7())

	l, err := oplog.New(pageID, actor, oplog.ActorUser, nil, nil,
		pageop.Text{BlockID: documentcore.BlockID(uuid.Must(uuid.NewV7())), Op: ops.InsertText{At: nil, Text: "hi"}})
	require.NoError(t, err)
	_, err = repo.Append(context.Background(), l)
	require.NoError(t, err)

	_, err = Trace(context.Background(), pageID, repo, "server-actor")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknownBlock)
}
