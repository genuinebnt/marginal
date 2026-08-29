package mdc

import (
	"fmt"
	"time"
)

// Diagnostic is something odd about the input that did not stop the compile.
//
// Compile returns (tree, diagnostics), never an error for malformed input.
// A paste with a broken fence must still produce a document: there is no
// "the paste failed" state that is useful to a person — there is a document,
// and a list of things that were odd about it.
type Diagnostic struct {
	Message string `json:"message"`
	// Line is 1-based, 0 when the diagnostic is about the whole input.
	Line int `json:"line"`
}

// Stats are what § 11 prints. Every one is measured on the call that returned
// them; none is quoted from a benchmark.
type Stats struct {
	Chars int `json:"chars"`
	Bytes int `json:"bytes"`
	// Divergences are the characters where the byte count and the rune count
	// part company — the reason offsets in this system are bytes, made
	// visible rather than argued for.
	Divergences []string `json:"divergences"`
	Tokens      int      `json:"tokens"`
	Blocks      int      `json:"blocks"`
	Ops         int      `json:"ops"`
	LexNs       int64    `json:"lex_ns"`
	ParseNs     int64    `json:"parse_ns"`
	EmitNs      int64    `json:"emit_ns"`
	ReplayNs    int64    `json:"replay_ns"`
}

// Result is the whole pipeline, every stage kept.
//
// The stages are returned rather than only the last one because § 11 draws
// all five, and because a pipeline whose intermediate forms are unreachable
// is a pipeline you cannot debug — the screen exists to make each arrow
// inspectable.
type Result struct {
	Tokens []Token `json:"tokens"`
	Tree   Tree    `json:"tree"`
	Ops    []Op    `json:"ops"`
	// Replayed is Tree rebuilt from Ops. Holds is whether the two matched.
	Replayed Tree   `json:"replayed"`
	Holds    bool   `json:"holds"`
	Mismatch string `json:"mismatch,omitempty"`

	Diagnostics []Diagnostic `json:"diagnostics"`
	Stats       Stats        `json:"stats"`
}

// Compile runs the whole pipeline and CHECKS the round-trip property on every
// call.
//
// That check is not a debug affordance. § 11's header says HOLDS because this
// comparison happened — replaying the emitted ops into an empty tree produced
// the same tree the pipeline built directly — and a screen that asserted it
// instead would be a screen that cannot be caught being wrong.
//
// idFor makes the block ids deterministic per call so two Compiles of the
// same input are equal, which is what lets a test compare results directly
// and what stops the lab screen from re-rendering every id on every keystroke.
func Compile(src string) Result {
	t0 := time.Now()
	tokens := Lex(src)
	lexNs := time.Since(t0).Nanoseconds()

	t1 := time.Now()
	tree, diags := lower(tokens)
	parseNs := time.Since(t1).Nanoseconds()

	t2 := time.Now()
	ops := Emit(tree)
	emitNs := time.Since(t2).Nanoseconds()

	t3 := time.Now()
	replayed, err := Replay(ops)
	replayNs := time.Since(t3).Nanoseconds()

	holds, mismatch := false, ""
	if err != nil {
		mismatch = err.Error()
	} else {
		holds, mismatch = tree.Equal(replayed)
	}

	return Result{
		Tokens:      tokens,
		Tree:        tree,
		Ops:         ops,
		Replayed:    replayed,
		Holds:       holds,
		Mismatch:    mismatch,
		Diagnostics: diags,
		Stats: Stats{
			Chars:       CountChars(src),
			Bytes:       len(src),
			Divergences: divergences(src),
			Tokens:      len(tokens),
			Blocks:      len(tree.Blocks),
			Ops:         len(ops),
			LexNs:       lexNs,
			ParseNs:     parseNs,
			EmitNs:      emitNs,
			ReplayNs:    replayNs,
		},
	}
}

// divergences lists the distinct multi-byte characters in the input, capped.
// § 11 prints them beside CHARS and BYTES so the gap between the two has a
// cause on screen rather than being a number to take on trust.
func divergences(src string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, r := range src {
		if r < 128 {
			continue
		}
		s := string(r)
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
		if len(out) == 6 {
			break
		}
	}
	return out
}

// Emit turns the tree into ops in an order every op can be applied in.
//
// INVARIANT E1 — emission order is a valid application order: a block's
// InsertBlock precedes any op naming it, and `after` names a sibling that
// already exists. Replay is what checks it, on every Compile.
//
// The tree is already depth-first, so this is one pass: `after` is the
// previous sibling under the same parent, or nil for a first child.
func Emit(tree Tree) []Op {
	lastSibling := map[string]string{} // parent → last emitted child id
	ops := make([]Op, 0, len(tree.Blocks))

	for _, b := range tree.Blocks {
		var parent, after *string
		if b.Parent != "" {
			p := b.Parent
			parent = &p
		}
		if prev, ok := lastSibling[b.Parent]; ok {
			a := prev
			after = &a
		}
		lastSibling[b.Parent] = b.ID

		ops = append(ops, Op{
			Scope: "block", Type: "InsertBlock",
			ID: b.ID, Parent: parent, After: after,
			Kind: b.Kind, Content: b.Content,
		})
	}
	return ops
}

// idFor produces stable, readable ids. Deterministic per compile so the same
// input always yields the same result — a lab screen whose ids churn on every
// keystroke is one that re-renders everything on every keystroke.
//
// These are NOT the ids the editor will use: paste rewrites them to UUIDs at
// the boundary, because a block id has to be unique across a workspace and
// "b3" is not. Kept readable here because § 11 prints them.
func idFor(n int) string { return fmt.Sprintf("b%d", n) }
