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

func TestItemAtSkipsALeadingTombstoneRun(t *testing.T) {
	// The exact scenario that broke a first attempt at ItemAt (reusing
	// sliceIndexForLiveOffset, whose "insertion point before the pos-th
	// live item" contract can land on a preceding tombstoned run): three
	// items, the first tombstoned, and ItemAt(0) must return the second
	// item's id — the actual first LIVE item — not the tombstoned one
	// sitting at slice index 0.
	log := NewLog()
	gen := NewIDGenerator("actor-1")
	ids := idsN(gen, 3)
	require.NoError(t, log.InsertAt(0, ids))
	require.NoError(t, log.Tombstone(0, 1)) // tombstone the first item only

	got, err := log.ItemAt(0)
	require.NoError(t, err)
	assert.Equal(t, ids[1], got, "ItemAt(0) must be the first LIVE item, not the tombstoned one at slice index 0")

	got, err = log.ItemAt(1)
	require.NoError(t, err)
	assert.Equal(t, ids[2], got)
}

func TestItemAtOutOfBounds(t *testing.T) {
	tests := []struct {
		name    string
		insertN int
		index   int
	}{
		{name: "index past end", insertN: 2, index: 2},
		{name: "negative index", insertN: 2, index: -1},
		{name: "empty log", insertN: 0, index: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			log := NewLog()
			gen := NewIDGenerator("actor-1")
			if tc.insertN > 0 {
				require.NoError(t, log.InsertAt(0, idsN(gen, tc.insertN)))
			}

			_, err := log.ItemAt(tc.index)
			assert.ErrorIs(t, err, ErrOutOfBounds)
		})
	}
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
