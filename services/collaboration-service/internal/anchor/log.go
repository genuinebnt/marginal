package anchor

import (
	"errors"
	"slices"
)

var ErrOutOfBounds = errors.New("anchor: offset out of bounds")

// item is one character's identity and life state, in document order.
// Tombstoned items are kept, never removed — deleting the slice entry
// would forget the very thing Resolve needs to detect and detach from.
type item struct {
	id         ItemID
	tombstoned bool
}

// Log tracks every character ever inserted into one Text (live or
// tombstoned) in document order, and answers "what live offset is this
// ItemID at now" — the rope holds the content, Log holds the identities;
// see the package doc comment on why they're separate structures.
//
// This is a correctness-first version: every operation is still O(n) in
// the number of items the Log has ever seen (liveCountBefore/LiveLen/
// ItemAt/sliceIndexForLiveOffset are all linear scans). Deliberate, per
// .agents/agents.md §2 ("ship minimal, refactor on friction") — a page's
// text is not expected to reach a size where this matters for a demo, and
// the fix when it does is well-known and localized: replace the linear
// liveCountBefore scan with an order-statistics structure (a Fenwick/BIT
// tree over "is this item live", giving O(log n) rank queries) without
// changing Log's exported API at all. Optimize only after that's
// actually measured, not before. index itself no longer rebuilds wholesale
// on every InsertAt (reindexFrom updates only the shifted suffix) — a
// narrower fix than the O(n) scans above, done because throwing away and
// rebuilding an entire map that's almost entirely still correct crossed
// from "simple" into "wasteful for no simplicity gained," not because the
// broader O(n) design above was reconsidered.
type Log struct {
	items []item
	index map[ItemID]int // id -> current slice index; rebuilt on mutation
}

func NewLog() *Log {
	return &Log{index: make(map[ItemID]int)}
}

// Len is the total item count, live and tombstoned.
func (l *Log) Len() int { return len(l.items) }

// LiveLen is the count of live items — what a Rope built from the same
// edit sequence should report for its own Len().
func (l *Log) LiveLen() int {
	n := 0
	for _, it := range l.items {
		if !it.tombstoned {
			n++
		}
	}
	return n
}

// InsertAt inserts ids as new, live items starting at live offset pos
// (i.e. pos counts only live items before it, the same indexing a Rope
// built from the identical edit sequence uses).
func (l *Log) InsertAt(pos int, ids []ItemID) error {
	sliceIdx, err := l.sliceIndexForLiveOffset(pos)
	if err != nil {
		return err
	}

	inserted := make([]item, len(ids))
	for i, id := range ids {
		inserted[i] = item{id: id}
	}

	l.items = slices.Insert(l.items, sliceIdx, inserted...)
	l.reindexFrom(sliceIdx)
	return nil
}

// Tombstone marks the items in the live range [start, end) as deleted —
// kept in the Log, just no longer live, so an Anchor pointing at one of
// them can still Detach to the nearest live position instead of
// resolving to Unknown.
func (l *Log) Tombstone(start, end int) error {
	if start > end {
		return ErrOutOfBounds
	}
	if end > l.LiveLen() {
		// Checked up front, before anything is mutated — an earlier
		// version discovered an out-of-range end only after already
		// tombstoning every live item from start to the end of the
		// slice, and returned that error with the Log left partially
		// mutated: a method that fails should leave its receiver
		// exactly as it found it, not sometimes.
		return ErrOutOfBounds
	}
	sliceIdx, err := l.sliceIndexForLiveOffset(start)
	if err != nil {
		return err
	}

	remaining := end - start
	for i := sliceIdx; i < len(l.items) && remaining > 0; i++ {
		if !l.items[i].tombstoned {
			l.items[i].tombstoned = true
			remaining--
		}
	}
	return nil
}

// Resolve finds a's current position. See ResolvedKind's doc comments for
// what each outcome means.
func (l *Log) Resolve(a Anchor) Resolved {
	idx, ok := l.index[a.Item]
	if !ok {
		return Resolved{Kind: Unknown}
	}

	before := l.liveCountBefore(idx)
	if !l.items[idx].tombstoned {
		pos := before
		if a.Bias == After {
			pos++
		}
		return Resolved{Kind: At, Offset: pos}
	}
	return Resolved{Kind: Detached, Offset: before}
}

// ItemAt returns the ItemID of the pos-th live item (0-indexed) — the
// reverse of Resolve: "what identity lives at this offset" instead of
// "what offset does this identity live at now." A client that only ever
// sees rune offsets (a plain textarea, not this package's own Anchor
// type) needs this to construct an Anchor for a position it observed
// rather than one it already held an identity for — internal/ops has no
// such caller yet (every op it builds already carries an Anchor from a
// prior Resolve), but internal/wsapi's document-boundary lookup does.
//
// Deliberately its own scan, not a reuse of sliceIndexForLiveOffset:
// that method's contract is "the insertion point before the pos-th live
// item," which can legitimately land on a preceding tombstoned run — the
// right answer for InsertAt, the wrong one here, where the item at the
// returned index must itself be live.
func (l *Log) ItemAt(pos int) (ItemID, error) {
	if pos < 0 {
		return ItemID{}, ErrOutOfBounds
	}
	live := 0
	for _, it := range l.items {
		if it.tombstoned {
			continue
		}
		if live == pos {
			return it.id, nil
		}
		live++
	}
	return ItemID{}, ErrOutOfBounds
}

// sliceIndexForLiveOffset finds the slice index of the pos-th live item
// (0-indexed), or len(items) if pos equals the current live count
// (inserting at the very end).
func (l *Log) sliceIndexForLiveOffset(pos int) (int, error) {
	if pos < 0 {
		return 0, ErrOutOfBounds
	}
	live := 0
	for i, it := range l.items {
		if live == pos {
			return i, nil
		}
		if !it.tombstoned {
			live++
		}
	}
	if live == pos {
		return len(l.items), nil
	}
	return 0, ErrOutOfBounds
}

func (l *Log) liveCountBefore(idx int) int {
	n := 0
	for i := 0; i < idx; i++ {
		if !l.items[i].tombstoned {
			n++
		}
	}
	return n
}

// reindexFrom updates l.index for every item from slice index start
// onward — everything before start kept its index, since InsertAt (the
// only caller) only ever grows the slice at or after start. Replaces an
// earlier full l.index = make(...) + rebuild-every-entry on every single
// insert, which threw away a map that was almost entirely still correct.
func (l *Log) reindexFrom(start int) {
	for i := start; i < len(l.items); i++ {
		l.index[l.items[i].id] = i
	}
}
