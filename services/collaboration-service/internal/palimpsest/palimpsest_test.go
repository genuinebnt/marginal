package palimpsest

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/documentcore"

	"marginal/collaboration-service/internal/anchor"
	"marginal/collaboration-service/internal/oplog"
	"marginal/collaboration-service/internal/ops"
	"marginal/collaboration-service/internal/pageop"
)

func mustClock() oplog.VectorClock { return oplog.VectorClock{} }

// step builds one confirmed LoggedOp wrapping op for blockID, authored
// by actor — this package's tests only care about Op/ActorID/BlockID, so
// every other LoggedOp field is left at its zero value.
func step(t *testing.T, actor uuid.UUID, blockID documentcore.BlockID, op ops.Op) oplog.LoggedOp {
	t.Helper()
	l, err := oplog.New(uuid.Nil, actor, oplog.ActorUser, nil, mustClock(), pageop.Text{BlockID: blockID, Op: op})
	require.NoError(t, err)
	return l
}

// TestBuildKeepsDeletedCharactersTombstonedNotRemoved is the package's
// whole point, pinned directly: "hi" then deleting "h" must leave BOTH
// characters in the returned array — 'h' tombstoned, 'i' still live —
// never shrink back to length 1.
func TestBuildKeepsDeletedCharactersTombstonedNotRemoved(t *testing.T) {
	blockID := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	actor := uuid.Must(uuid.NewV7())

	confirmed := []oplog.LoggedOp{
		step(t, actor, blockID, ops.InsertText{At: nil, Text: "hi"}),
	}
	chars, err := Build(confirmed, blockID, "palimpsest")
	require.NoError(t, err)
	require.Len(t, chars, 2)
	firstItem := anchor.ItemID{Actor: "palimpsest", Counter: 1} // 'h' — the insert-order identity Build's internal doctext.Text assigns

	confirmed = append(confirmed, step(t, actor, blockID, ops.DeleteText{
		Range: anchor.AnchorRange{
			Start: anchor.Anchor{Item: firstItem, Bias: anchor.Before},
			End:   anchor.Anchor{Item: firstItem, Bias: anchor.After},
		},
	}))
	chars, err = Build(confirmed, blockID, "palimpsest")
	require.NoError(t, err)
	require.Len(t, chars, 2, "the deleted character must still be in the array — tombstoned, not removed")

	assert.Equal(t, 'h', chars[0].Rune)
	assert.Equal(t, 0, chars[0].InsertStep)
	require.NotNil(t, chars[0].DeleteStep, "'h' was deleted at step 1")
	assert.Equal(t, 1, *chars[0].DeleteStep)
	require.NotNil(t, chars[0].DeleteActor)
	assert.Equal(t, actor, *chars[0].DeleteActor)

	assert.Equal(t, 'i', chars[1].Rune)
	assert.Nil(t, chars[1].DeleteStep, "'i' was never deleted")

	assert.Equal(t, "i", LiveText(chars), "the live-text projection must agree with an ordinary doctext replay")
}

// TestAtVersionFiltersByInsAndDel pins the mockup's own filter:
// "reading version v is the filter ins <= v < del."
func TestAtVersionFiltersByInsAndDel(t *testing.T) {
	blockID := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	actor := uuid.Must(uuid.NewV7())

	confirmed := []oplog.LoggedOp{
		step(t, actor, blockID, ops.InsertText{At: nil, Text: "ab"}), // step 0: a, b
	}
	firstItem := anchor.ItemID{Actor: "palimpsest", Counter: 1} // 'a'
	confirmed = append(confirmed,
		step(t, actor, blockID, ops.DeleteText{ // step 1: delete 'a'
			Range: anchor.AnchorRange{
				Start: anchor.Anchor{Item: firstItem, Bias: anchor.Before},
				End:   anchor.Anchor{Item: firstItem, Bias: anchor.After},
			},
		}),
	)
	chars, err := Build(confirmed, blockID, "palimpsest")
	require.NoError(t, err)
	require.Len(t, chars, 2)

	atStep0 := AtVersion(chars, 0)
	require.Len(t, atStep0, 2, "right after step 0, both a and b are live")
	assert.Equal(t, 'a', atStep0[0].Rune)
	assert.Equal(t, 'b', atStep0[1].Rune)

	atStep1 := AtVersion(chars, 1)
	require.Len(t, atStep1, 1, "right after step 1, only b survives — a is tombstoned as of exactly this step")
	assert.Equal(t, 'b', atStep1[0].Rune)
}

// TestBuildIgnoresOpsForOtherBlocksAndTiers is what makes this package
// safe to point at a real page's whole confirmed log rather than a
// pre-filtered slice — a real page interleaves many blocks' own ops plus
// structural (pageop.Block) ops.
func TestBuildIgnoresOpsForOtherBlocksAndTiers(t *testing.T) {
	blockID := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	otherBlock := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	actor := uuid.Must(uuid.NewV7())

	structural, err := oplog.New(uuid.Nil, actor, oplog.ActorUser, nil, mustClock(), pageop.Block{
		Op: documentcore.InsertBlock{ID: blockID, Kind: documentcore.NewParagraph(), Content: documentcore.Content{Text: ""}},
	})
	require.NoError(t, err)

	confirmed := []oplog.LoggedOp{
		structural,
		step(t, actor, otherBlock, ops.InsertText{At: nil, Text: "not this block"}),
		step(t, actor, blockID, ops.InsertText{At: nil, Text: "yes"}),
	}
	chars, err := Build(confirmed, blockID, "palimpsest")
	require.NoError(t, err)
	require.Len(t, chars, 3)
	assert.Equal(t, "yes", LiveText(chars))
	assert.Equal(t, 2, chars[0].InsertStep, "must record the confirmed log's own step index, not a re-based count")
}
