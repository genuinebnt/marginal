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
	"marginal/collaboration-service/internal/ops"
	"marginal/collaboration-service/internal/pageop"
)

// TestReplayAcrossRestartRequiresAStableServerActor is a real,
// live-discovered bug, pinned directly: anchor.ItemID{Actor, Counter} is
// embedded in every persisted op that names an anchor (InsertText's own
// "at" anchor, DeleteText's Range), because applyBlockOp/syncBlockContent
// build every block's doctext.Text via doctext.New(serverActor) — not
// the editing user's own actor id (RFC-002's own "ItemID is about
// ordering, not authorship"). If a later replay (this test: a fresh
// open() standing in for a process restart) uses a DIFFERENT serverActor
// than the one that originally created those items, the freshly-replayed
// anchor.Log assigns different ids for the same characters, and any op
// that names an anchor by its ORIGINAL id (a DeleteText, or an
// InsertText anchored to a prior insert's own boundary — exactly what
// live typing produces) fails to resolve. cmd/main.go's own serverActor
// must therefore be a stable, fixed value across restarts, never
// regenerated per process start — this test is what proves that
// requirement is real, not defensive.
func TestReplayAcrossRestartRequiresAStableServerActor(t *testing.T) {
	repo := newFakeRepo()
	pageID := uuid.Must(uuid.NewV7())
	actor := uuid.Must(uuid.NewV7())
	ctx := context.Background()

	// First "process": commits a real character-edit history whose later
	// steps anchor to earlier ones — insert "Hello", then insert " world"
	// anchored to the first insert's own boundary, then delete it all.
	// This is exactly the shape live typing produces (RichEditorPane
	// always anchors its next InsertText to the block's current
	// Boundaries), and exactly the shape that needs anchor resolution
	// across steps to replay at all.
	first, err := open(ctx, pageID, repo, t.TempDir(), "server", nil,
		[]flush.Option{flush.WithBatchSize(1), flush.WithInterval(time.Hour)}, nil)
	require.NoError(t, err)

	blockID := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	_, err = first.ApplyClientOp(ctx, actor, oplog.ActorUser, pageop.Block{
		Op: documentcore.InsertBlock{ID: blockID, Kind: documentcore.NewParagraph(), Content: documentcore.Content{Text: ""}},
	}, nil, 0)
	require.NoError(t, err)

	result, err := first.ApplyClientOp(ctx, actor, oplog.ActorUser, pageop.Text{BlockID: blockID, Op: ops.InsertText{At: nil, Text: "Hello"}}, nil, 0)
	require.NoError(t, err)
	boundaries := result.Boundaries
	require.NotNil(t, boundaries, "a non-empty block must report its boundaries")

	_, err = first.ApplyClientOp(ctx, actor, oplog.ActorUser, pageop.Text{BlockID: blockID, Op: ops.InsertText{At: &boundaries.End, Text: " world"}}, nil, 0)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		got, _ := repo.ListForPage(ctx, pageID)
		return len(got) == 3
	}, time.Second, time.Millisecond)
	require.NoError(t, first.Close())

	// Second "process," same serverActor ("server") — the fixed-value
	// fix. Replay must succeed, and the block's live text must be
	// exactly what the first process left it as.
	second, err := open(ctx, pageID, repo, t.TempDir(), "server", nil, noAutoFlush(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })
	assert.Equal(t, "Hello world", blockText(t, second, blockID))

	// Third "process," a DIFFERENT serverActor — the exact regression:
	// this must fail to open, the same "anchor refers to an item this
	// text never saw" error a real restart with a randomized serverActor
	// actually produced live.
	_, err = open(ctx, pageID, repo, t.TempDir(), "a-different-server-actor", nil, noAutoFlush(), nil)
	require.Error(t, err, "replaying with a different serverActor than originally created these anchors must fail — pinning why serverActor must be stable, not regenerated per restart")
}
