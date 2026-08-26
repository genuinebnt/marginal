package anchor

import (
	"testing"

	"pgregory.net/rapid"
)

// refItem mirrors Log's internal item, kept as a separate, obviously-correct
// model to differentially test against — same approach as rope's
// TestPropertyMatchesNaiveStringReference.
type refItem struct {
	id         ItemID
	tombstoned bool
}

func refLiveCountBefore(items []refItem, idx int) int {
	n := 0
	for i := 0; i < idx; i++ {
		if !items[i].tombstoned {
			n++
		}
	}
	return n
}

func refInsertAt(items []refItem, pos int, ids []ItemID) ([]refItem, bool) {
	live := 0
	sliceIdx := -1
	for i, it := range items {
		if live == pos {
			sliceIdx = i
			break
		}
		if !it.tombstoned {
			live++
		}
	}
	if sliceIdx == -1 {
		if live != pos {
			return items, false
		}
		sliceIdx = len(items)
	}
	inserted := make([]refItem, len(ids))
	for i, id := range ids {
		inserted[i] = refItem{id: id}
	}
	out := make([]refItem, 0, len(items)+len(ids))
	out = append(out, items[:sliceIdx]...)
	out = append(out, inserted...)
	out = append(out, items[sliceIdx:]...)
	return out, true
}

func refTombstone(items []refItem, start, end int) ([]refItem, bool) {
	if start > end {
		return items, false
	}
	live := 0
	sliceIdx := -1
	for i, it := range items {
		if live == start {
			sliceIdx = i
			break
		}
		if !it.tombstoned {
			live++
		}
	}
	if sliceIdx == -1 {
		if live != start {
			return items, false
		}
		sliceIdx = len(items)
	}
	remaining := end - start
	out := make([]refItem, len(items))
	copy(out, items)
	for i := sliceIdx; i < len(out) && remaining > 0; i++ {
		if !out[i].tombstoned {
			out[i].tombstoned = true
			remaining--
		}
	}
	return out, remaining == 0
}

// TestPropertyLogMatchesReferenceModel drives Log and a plain reference
// slice through the same random sequence of inserts/tombstones and
// requires every still-known item's resolved position to agree.
func TestPropertyLogMatchesReferenceModel(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		log := NewLog()
		gen := NewIDGenerator("actor")
		var ref []refItem
		var allIDs []ItemID

		steps := rapid.IntRange(1, 40).Draw(t, "steps")
		for i := 0; i < steps; i++ {
			liveLen := 0
			for _, it := range ref {
				if !it.tombstoned {
					liveLen++
				}
			}

			doInsert := liveLen == 0 || rapid.Bool().Draw(t, "insert")
			if doInsert {
				pos := rapid.IntRange(0, liveLen).Draw(t, "pos")
				n := rapid.IntRange(1, 3).Draw(t, "n")
				ids := idsN(gen, n)
				allIDs = append(allIDs, ids...)

				if err := log.InsertAt(pos, ids); err != nil {
					t.Fatalf("Log.InsertAt(%d, %d ids) failed: %v", pos, n, err)
				}
				newRef, ok := refInsertAt(ref, pos, ids)
				if !ok {
					t.Fatalf("reference insert at %d rejected unexpectedly", pos)
				}
				ref = newRef
			} else {
				a := rapid.IntRange(0, liveLen-1).Draw(t, "a")
				b := rapid.IntRange(a+1, liveLen).Draw(t, "b")

				if err := log.Tombstone(a, b); err != nil {
					t.Fatalf("Log.Tombstone(%d, %d) failed: %v", a, b, err)
				}
				newRef, ok := refTombstone(ref, a, b)
				if !ok {
					t.Fatalf("reference tombstone [%d,%d) rejected unexpectedly", a, b)
				}
				ref = newRef
			}

			refLive := 0
			for _, it := range ref {
				if !it.tombstoned {
					refLive++
				}
			}
			if log.LiveLen() != refLive {
				t.Fatalf("LiveLen mismatch: log=%d ref=%d", log.LiveLen(), refLive)
			}

			for _, id := range allIDs {
				idx := -1
				for i, it := range ref {
					if it.id == id {
						idx = i
						break
					}
				}
				if idx == -1 {
					continue
				}
				wantBefore := refLiveCountBefore(ref, idx)
				got := log.Resolve(Anchor{Item: id, Bias: Before})
				if ref[idx].tombstoned {
					if got.Kind != Detached || got.Offset != wantBefore {
						t.Fatalf("Resolve(%v) = %+v, want Detached{%d}", id, got, wantBefore)
					}
				} else {
					if got.Kind != At || got.Offset != wantBefore {
						t.Fatalf("Resolve(%v) = %+v, want At{%d}", id, got, wantBefore)
					}
				}
			}
		}
	})
}
