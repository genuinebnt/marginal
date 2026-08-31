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
func TestBuildIgnoresOpsNamingOtherBlocks(t *testing.T) {
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

// blockStep wraps a structural op the same way step wraps a character one.
func blockStep(t *testing.T, actor uuid.UUID, op documentcore.Op) oplog.LoggedOp {
	t.Helper()
	l, err := oplog.New(uuid.Nil, actor, oplog.ActorUser, nil, mustClock(), pageop.Block{Op: op})
	require.NoError(t, err)
	return l
}

// A real, live-discovered bug, pinned directly.
//
// session.applyBlockOp SEEDS a block's rope from InsertBlock.Content.Text,
// so every character a block is born with is an item only that seeding
// created. Build used to skip every structural op — which meant the anchors
// in any later DeleteText named items its own replay had never made, and it
// failed with "anchor refers to an item this text never saw".
//
// It went unnoticed for as long as no block was ever both created with text
// AND character-edited afterwards; the seeded corpus wrote text through
// InsertBlock and never typed into it again, so the panel was empty rather
// than broken, which looks the same.
func TestBuildReplaysTheTextABlockWasBornWith(t *testing.T) {
	blockID := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	actor := uuid.Must(uuid.NewV7())

	confirmed := []oplog.LoggedOp{
		blockStep(t, actor, documentcore.InsertBlock{
			ID: blockID, Kind: documentcore.NewParagraph(),
			Content: documentcore.Content{Text: "born"},
		}),
	}
	chars, err := Build(confirmed, blockID, "palimpsest")
	require.NoError(t, err)
	require.Len(t, chars, 4, "the text a block is created with is character history like any other")
	assert.Equal(t, "born", LiveText(chars))

	// And the anchors that seeding produced must resolve, which is the half
	// that was actually failing: delete "bo" by naming those very items.
	confirmed = append(confirmed, step(t, actor, blockID, ops.DeleteText{Range: anchor.AnchorRange{
		Start: anchor.Anchor{Item: anchor.ItemID{Actor: "palimpsest", Counter: 1}, Bias: anchor.Before},
		End:   anchor.Anchor{Item: anchor.ItemID{Actor: "palimpsest", Counter: 2}, Bias: anchor.After},
	}}))
	chars, err = Build(confirmed, blockID, "palimpsest")
	require.NoError(t, err)
	require.Len(t, chars, 4, "a delete tombstones, it never shrinks the array")
	assert.Equal(t, "rn", LiveText(chars))
	require.NotNil(t, chars[0].DeleteStep)
	assert.Equal(t, 1, *chars[0].DeleteStep)
}

// A wholesale reseed is a delete-everything-then-insert as far as character
// history goes, and has to be recorded as exactly that — otherwise the
// characters it replaced vanish from a structure whose entire promise is
// that nothing ever vanishes from it.
func TestSetBlockContentTombstonesWhatItReplaced(t *testing.T) {
	blockID := documentcore.BlockID(uuid.Must(uuid.NewV7()))
	first := uuid.Must(uuid.NewV7())
	second := uuid.Must(uuid.NewV7())

	confirmed := []oplog.LoggedOp{
		blockStep(t, first, documentcore.InsertBlock{
			ID: blockID, Kind: documentcore.NewParagraph(),
			Content: documentcore.Content{Text: "old"},
		}),
		blockStep(t, second, documentcore.SetBlockContent{
			Block:   blockID,
			Prev:    documentcore.Content{Text: "old"},
			Content: documentcore.Content{Text: "new"},
		}),
	}
	chars, err := Build(confirmed, blockID, "palimpsest")
	require.NoError(t, err)
	require.Len(t, chars, 6, "three replaced characters plus three new ones")
	assert.Equal(t, "new", LiveText(chars))

	for i := 0; i < 3; i++ {
		require.NotNilf(t, chars[i].DeleteStep, "char %d of the replaced text is still marked live", i)
		assert.Equal(t, 1, *chars[i].DeleteStep)
		require.NotNil(t, chars[i].DeleteActor)
		assert.Equal(t, second, *chars[i].DeleteActor, "the replacement is attributed to whoever sent it")
	}
	for i := 3; i < 6; i++ {
		assert.Nil(t, chars[i].DeleteStep, "the new text is live")
		assert.Equal(t, second, chars[i].InsertActor)
	}
}
