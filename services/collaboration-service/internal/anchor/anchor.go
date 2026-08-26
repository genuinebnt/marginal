// Package anchor is RFC-001 §9's stable-position scheme: a plain integer
// offset into live text doesn't survive a concurrent edit — if a mark is
// anchored at "offset 5" and someone else inserts 3 characters at offset
// 2 first, the mark now points at the wrong character. An Anchor instead
// names a specific character by its permanent identity (ItemID, assigned
// once at insertion, never reused or reassigned) plus a Bias saying which
// side of that character the position sits on, and is resolved back to a
// current offset on demand — RFC-001 §9: "marks persist as byte offsets
// in JSONB (regenerable); comment anchors persist as item ids (must
// outlive a session)."
//
// collaboration-service is a single doc-actor per page (ARCHITECTURE.md:
// "one document, one owner, at any time") — every concurrent client's op
// is applied by that one process in some serial order, not merged
// peer-to-peer across replicas the way Yjs/Automerge's offline-first CRDTs
// are. That's why this package is scoped to resolution against one
// authoritative sequence, not multi-replica merge: an op composed against
// an older position resolves its Anchors against whatever the document
// looks like *now*, and that's the whole mechanism this needs.
package anchor

import (
	"encoding/json"
	"fmt"
)

// ItemID is a Lamport-style identity: which actor, and that actor's own
// monotonic counter when it was assigned. Two actors can never produce
// the same ItemID, and one actor's ids are totally ordered by Counter.
type ItemID struct {
	Actor   string `json:"actor"`
	Counter uint64 `json:"counter"`
}

// IDGenerator hands out this actor's next ItemID. Not safe for concurrent
// use by multiple goroutines — collaboration-service's doc-actor model
// means exactly one goroutine owns a given page's state at a time.
type IDGenerator struct {
	actor   string
	counter uint64
}

func NewIDGenerator(actor string) *IDGenerator { return &IDGenerator{actor: actor} }

func (g *IDGenerator) Next() ItemID {
	g.counter++
	return ItemID{Actor: g.actor, Counter: g.counter}
}

// Bias disambiguates which side of Item an Anchor sits on — needed
// because "the position right before this character" and "the position
// right after the previous character" are the same offset today but can
// diverge once more edits land around them.
type Bias int

const (
	Before Bias = iota
	After
)

// MarshalJSON/UnmarshalJSON render Bias as "before"/"after" rather than a
// raw 0/1 — this type crosses the browser-facing wire (collaboration's
// WebSocket contract), and a named value is worth the few extra bytes
// where documentcore's own wire types (BlockKind, MarkKind) already made
// the same call.
func (b Bias) MarshalJSON() ([]byte, error) {
	switch b {
	case Before:
		return []byte(`"before"`), nil
	case After:
		return []byte(`"after"`), nil
	default:
		return nil, fmt.Errorf("anchor: unknown Bias %d", b)
	}
}

func (b *Bias) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch s {
	case "before":
		*b = Before
	case "after":
		*b = After
	default:
		return fmt.Errorf("anchor: unknown Bias %q", s)
	}
	return nil
}

// Anchor is a stable position: stick to Item, on the Bias side of it.
type Anchor struct {
	Item ItemID `json:"item"`
	Bias Bias   `json:"bias"`
}

// AnchorRange is a stable range — what a mark or comment's extent is
// actually persisted as (RFC-001 §9), since a plain [start,end) offset
// pair has the same problem a single offset does.
type AnchorRange struct {
	Start Anchor `json:"start"`
	End   Anchor `json:"end"`
}

// ResolvedKind is Resolve's outcome.
type ResolvedKind int

const (
	// At: Item is live; Offset is its current position.
	At ResolvedKind = iota
	// Detached: Item was deleted since the Anchor was created; Offset is
	// where the gap it left behind currently is — the nearest live
	// position, per RFC-001 §9's Resolved::Detached{nearest_live}.
	Detached
	// Unknown: no such Item exists in this Log at all (never inserted
	// here, or a different actor/page entirely) — a real error, not a
	// deletion.
	Unknown
)

type Resolved struct {
	Kind   ResolvedKind
	Offset int // meaningful for At and Detached; 0 for Unknown
}
