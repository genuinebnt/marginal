package anchor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func idsN(gen *IDGenerator, n int) []ItemID {
	ids := make([]ItemID, n)
	for i := range ids {
		ids[i] = gen.Next()
	}
	return ids
}

func TestInsertThenResolveAt(t *testing.T) {
	log := NewLog()
	gen := NewIDGenerator("actor-1")
	ids := idsN(gen, 5) // represents "hello" at positions 0..4

	require.NoError(t, log.InsertAt(0, ids))
	assert.Equal(t, 5, log.LiveLen())

	got := log.Resolve(Anchor{Item: ids[2], Bias: Before})
	assert.Equal(t, Resolved{Kind: At, Offset: 2}, got)

	got = log.Resolve(Anchor{Item: ids[2], Bias: After})
	assert.Equal(t, Resolved{Kind: At, Offset: 3}, got)
}

func TestResolveUnknownItem(t *testing.T) {
	log := NewLog()
	got := log.Resolve(Anchor{Item: ItemID{Actor: "nobody", Counter: 1}})
	assert.Equal(t, ResolvedKind(Unknown), got.Kind)
}

func TestInsertAtMiddleShiftsLaterItems(t *testing.T) {
	log := NewLog()
	gen := NewIDGenerator("actor-1")
	original := idsN(gen, 5) // "hello"
	require.NoError(t, log.InsertAt(0, original))

	inserted := idsN(gen, 2) // insert "XY" at position 2: "heXYllo"
	require.NoError(t, log.InsertAt(2, inserted))

	assert.Equal(t, Resolved{Kind: At, Offset: 2}, log.Resolve(Anchor{Item: inserted[0], Bias: Before}))
	// original[2] ('l') was at 2, now shifted to 4 by the 2-char insert.
	assert.Equal(t, Resolved{Kind: At, Offset: 4}, log.Resolve(Anchor{Item: original[2], Bias: Before}))
}

func TestTombstonedItemDetachesToNearestLive(t *testing.T) {
	log := NewLog()
	gen := NewIDGenerator("actor-1")
	ids := idsN(gen, 5) // "hello"
	require.NoError(t, log.InsertAt(0, ids))

	// Delete "ell" (positions 1..4), leaving "ho".
	require.NoError(t, log.Tombstone(1, 4))
	assert.Equal(t, 2, log.LiveLen())

	got := log.Resolve(Anchor{Item: ids[2], Bias: Before}) // the deleted 'l'
	assert.Equal(t, Detached, got.Kind)
	assert.Equal(t, 1, got.Offset, "the gap left by the deletion is at live offset 1, right after 'h'")
}

func TestAnchorOnSurvivingItemResolvesCorrectlyAfterNeighborsDeleted(t *testing.T) {
	log := NewLog()
	gen := NewIDGenerator("actor-1")
	ids := idsN(gen, 5) // "hello"
	require.NoError(t, log.InsertAt(0, ids))

	require.NoError(t, log.Tombstone(0, 2)) // delete "he", leaving "llo"

	got := log.Resolve(Anchor{Item: ids[4], Bias: Before}) // the final 'o', still live
	assert.Equal(t, Resolved{Kind: At, Offset: 2}, got)
}

func TestInsertAtOutOfBoundsFails(t *testing.T) {
	log := NewLog()
	gen := NewIDGenerator("actor-1")
	err := log.InsertAt(5, idsN(gen, 1))
	assert.ErrorIs(t, err, ErrOutOfBounds)
}

func TestTombstoneOutOfBoundsFails(t *testing.T) {
	log := NewLog()
	gen := NewIDGenerator("actor-1")
	require.NoError(t, log.InsertAt(0, idsN(gen, 3)))
	err := log.Tombstone(1, 10)
	assert.ErrorIs(t, err, ErrOutOfBounds)
}

// TestConcurrentEditSimulation is the scenario Anchors exist for: two
// "clients" each compose an op against the same starting document, one
// gets applied first (shifting positions), and the second one's Anchor —
// composed before that shift happened — must still resolve to the
// character it actually meant, not the one that happens to sit at its
// original numeric offset now.
func TestConcurrentEditSimulation(t *testing.T) {
	log := NewLog()
	gen := NewIDGenerator("actor-1")
	ids := idsN(gen, 5) // "hello"
	require.NoError(t, log.InsertAt(0, ids))

	// Client B composes an anchor at "right before the 'l' at index 2"
	// against the current document.
	bAnchor := Anchor{Item: ids[2], Bias: Before}

	// Client A's concurrent insert lands first: "XY" at position 0.
	otherGen := NewIDGenerator("actor-2")
	aInserted := idsN(otherGen, 2)
	require.NoError(t, log.InsertAt(0, aInserted))
	require.Equal(t, "XYhello", stringifyForTest(log, ids, aInserted))

	// B's anchor must now resolve to offset 4 (shifted by A's 2-char
	// insert), not the stale offset 2 it was composed against.
	got := log.Resolve(bAnchor)
	assert.Equal(t, Resolved{Kind: At, Offset: 4}, got)
}

// stringifyForTest reconstructs a readable string from known id->char
// mappings, purely so TestConcurrentEditSimulation's intermediate state
// is easy to read in a failure message.
func stringifyForTest(log *Log, original, prefix []ItemID) string {
	chars := map[ItemID]byte{}
	for i, id := range original {
		chars[id] = "hello"[i]
	}
	for i, id := range prefix {
		chars[id] = "XY"[i]
	}
	out := make([]byte, 0, log.Len())
	for _, it := range log.items {
		if !it.tombstoned {
			out = append(out, chars[it.id])
		}
	}
	return string(out)
}
