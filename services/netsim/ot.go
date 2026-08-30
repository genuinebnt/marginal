// Package netsim is § 14 NETCODE's engine: two replicas, one server, one op
// log, and a wire you can make as bad as you like.
//
// It is a SIMULATION, and says so on the screen. `collaboration-service` is
// the real thing — a live rope per block, a real WAL, real sockets. What
// this module adds is the one thing the real service cannot give you: a
// deterministic, replayable 400 ms of a 4%-loss network, with prediction,
// rollback and transform all visible at once. Same laws, one actor, no
// clock.
//
// The text tier only. RFC-002 has two op tiers (block structure and
// characters within a block); transform on the block tier is
// `collaboration-service`'s own, and reimplementing it here would be a
// second implementation of the thing this screen exists to explain.
package netsim

import "fmt"

// Kind is the op tier this module simulates: characters within one block.
type Kind string

const (
	Insert Kind = "insert"
	Delete Kind = "delete"
)

// Op is one intent. Position is a byte offset into the block's text.
//
// `Base` is how many server ops the author had seen when they wrote it —
// the whole transform problem in one integer. An op with Base = the
// server's current length is not concurrent with anything and needs no
// transform; every op behind that is concurrent with the ops it missed.
type Op struct {
	ID    string `json:"id"`
	Actor string `json:"actor"`
	Kind  Kind   `json:"kind"`
	Pos   int    `json:"pos"`
	// Text for an insert; Len for a delete. Exactly one is meaningful.
	Text string `json:"text,omitempty"`
	Len  int    `json:"len,omitempty"`
	Base int    `json:"base"`
	// Deps is the op ids this one causally follows — every op its author
	// had seen. The DAG § 14 draws is built from these, not from time.
	Deps []string `json:"deps,omitempty"`
}

func (o Op) width() int {
	if o.Kind == Insert {
		return len(o.Text)
	}
	return o.Len
}

// Apply runs the op against a string, clamping rather than failing.
//
// Clamping is the honest behaviour here: a position past the end is what a
// MISSING transform produces, and this module's entire purpose is to let
// you turn transform off and watch that happen. Returning an error instead
// would hide the defect behind a stack trace.
func (o Op) Apply(s string) string {
	switch o.Kind {
	case Insert:
		p := clamp(o.Pos, 0, len(s))
		return s[:p] + o.Text + s[p:]
	case Delete:
		p := clamp(o.Pos, 0, len(s))
		e := clamp(p+o.Len, p, len(s))
		return s[:p] + s[e:]
	}
	return s
}

// Invert returns the op that undoes this one against the state it produced.
// Rollback is running inverses, never restoring a snapshot (RFC-002 §4) —
// the same rule the editor's undo already follows.
func (o Op) Invert(before string) Op {
	switch o.Kind {
	case Insert:
		return Op{ID: o.ID + "'", Actor: o.Actor, Kind: Delete, Pos: o.Pos, Len: len(o.Text)}
	case Delete:
		p := clamp(o.Pos, 0, len(before))
		e := clamp(p+o.Len, p, len(before))
		return Op{ID: o.ID + "'", Actor: o.Actor, Kind: Insert, Pos: o.Pos, Text: before[p:e]}
	}
	return o
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Transform rewrites `a` to apply after concurrent `b` has already applied —
// TP1, the inclusion transform.
//
// The tie-break on two inserts at the same position is by ACTOR NAME, not
// by arrival: arrival order differs per replica by definition, and a
// tie-break that differs per replica is exactly how two clients converge to
// two different documents. It has to be a total order both sides can
// compute alone.
func Transform(a, b Op) Op {
	switch {
	case a.Kind == Insert && b.Kind == Insert:
		if a.Pos < b.Pos || (a.Pos == b.Pos && a.Actor < b.Actor) {
			return a
		}
		a.Pos += len(b.Text)
		return a

	case a.Kind == Insert && b.Kind == Delete:
		switch {
		case a.Pos <= b.Pos:
			return a
		case a.Pos >= b.Pos+b.Len:
			a.Pos -= b.Len
		default:
			// Its anchor was concurrently deleted. The insertion is
			// CANCELLED — an empty insert, which Apply treats as the
			// no-op it is.
			//
			// This is the one genuinely lossy case in TP1 and the pair
			// below is why it has to be: to converge with single ops,
			// "insert inside a deleted range" must resolve the same way
			// from both sides, and the other side (delete vs insert)
			// can only either swallow the text or split into two
			// deletes. Splitting is not expressible as one op, so the
			// delete swallows and the insert cancels. Keeping the
			// insert here instead — collapsing it to b.Pos, which reads
			// more generous — makes the two paths disagree and the
			// replicas diverge. Verified by TestTransformConverges.
			//
			// The real service does not have to choose: it keeps
			// tombstones, so the deleted range still exists to anchor
			// against. This module models offsets, which is exactly
			// the design RFC-002 §3 rejected, and this is the cost.
			a.Pos = b.Pos
			a.Text = ""
		}
		return a

	case a.Kind == Delete && b.Kind == Insert:
		switch {
		case b.Pos <= a.Pos:
			a.Pos += len(b.Text)
		case b.Pos < a.Pos+a.Len:
			// Inserted inside the range being deleted: the delete
			// swallows it. See the note above — this and the insert
			// cancellation are one decision, not two.
			a.Len += len(b.Text)
		}
		return a

	default: // delete vs delete — subtract the overlap
		aEnd, bEnd := a.Pos+a.Len, b.Pos+b.Len
		switch {
		case bEnd <= a.Pos:
			a.Pos -= b.Len
		case b.Pos >= aEnd:
			// disjoint, b is after — unchanged
		default:
			overlap := min(aEnd, bEnd) - max(a.Pos, b.Pos)
			a.Len -= overlap
			if b.Pos < a.Pos {
				a.Pos = b.Pos
			}
			if a.Len < 0 {
				a.Len = 0
			}
		}
		return a
	}
}

func (o Op) String() string {
	if o.Kind == Insert {
		return fmt.Sprintf("%s ins @%d %q", o.Actor, o.Pos, o.Text)
	}
	return fmt.Sprintf("%s del @%d ×%d", o.Actor, o.Pos, o.Len)
}
