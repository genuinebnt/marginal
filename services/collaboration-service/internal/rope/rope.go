// Package rope is collaboration-service's live document text — RFC-001
// §2's "CRDT working format" side of the two-representations split
// (storage/wire is document-service's flat spans JSONB; this is what a
// live editing session actually mutates). A rope trades a plain string's
// O(n) insert/delete for O(log n), which matters here specifically
// because every keystroke in a live session is an insert or delete at an
// arbitrary offset — see docs/porting/BENCHMARKS.md for the measured
// difference against a naive string once this lands.
//
// Immutable/persistent, not mutate-in-place: Insert and Delete return a
// new Rope built by structural sharing with the old one, which is the
// idiomatic fit for Go here (no lifetime/ownership story to design around
// the way a Rust rope would need) and makes every operation trivially
// safe to reason about across goroutines without a lock — see
// .agents/agents.md §1.
package rope

import (
	"errors"
	"math/bits"
	"strings"
	"unicode/utf8"
)

// maxLeafBytes bounds how large a single leaf's text gets before New
// (initial construction) splits it — kept small enough that split/concat
// stay cheap, large enough that a typical rope isn't mostly tree
// overhead.
const maxLeafBytes = 512

// node is either a leaf (text set, left/right nil) or a branch. length
// and weight let split/index navigate without walking the whole subtree:
// weight is the left subtree's byte length, so "is offset i in the left
// half" is a single comparison at every level.
type node struct {
	text        string
	left, right *node
	weight      int
	length      int
	depth       int
}

func (n *node) isLeaf() bool { return n.left == nil && n.right == nil }

func newLeaf(s string) *node {
	if s == "" {
		return nil
	}
	return &node{text: s, weight: len(s), length: len(s)}
}

func newBranch(left, right *node) *node {
	return &node{
		left: left, right: right,
		weight: left.length,
		length: left.length + right.length,
		depth:  max(left.depth, right.depth) + 1,
	}
}

// Rope is immutable; the zero value is the empty rope.
type Rope struct{ root *node }

var (
	ErrOutOfBounds     = errors.New("rope: offset out of bounds")
	ErrNotCharBoundary = errors.New("rope: offset is not a char boundary")
	ErrInvertedRange   = errors.New("rope: start > end")
)

// New builds a Rope from s, splitting it into a balanced tree of leaves.
func New(s string) Rope {
	if s == "" {
		return Rope{}
	}
	return Rope{root: build(s)}
}

// build splits s into a balanced tree, dividing near the byte midpoint at
// a UTF-8 char boundary each time.
func build(s string) *node {
	if len(s) <= maxLeafBytes {
		return newLeaf(s)
	}
	mid := charBoundaryNear(s, len(s)/2)
	return newBranch(build(s[:mid]), build(s[mid:]))
}

// charBoundaryNear finds a UTF-8 rune-start position at or before at —
// used only to pick a safe split point near the middle, never to
// interpret user-supplied offsets (those go through validate, which
// rejects rather than silently adjusting).
func charBoundaryNear(s string, at int) int {
	for at > 0 && !utf8.RuneStart(s[at]) {
		at--
	}
	if at == 0 {
		// s[0] is always a boundary; this only triggers for a
		// pathological single leading multi-byte rune longer than the
		// search — fall back to the first rune's own length.
		_, size := utf8.DecodeRuneInString(s)
		return size
	}
	return at
}

func (r Rope) Len() int {
	if r.root == nil {
		return 0
	}
	return r.root.length
}

func (r Rope) String() string {
	if r.root == nil {
		return ""
	}
	var b strings.Builder
	b.Grow(r.root.length)
	writeString(&b, r.root)
	return b.String()
}

func writeString(b *strings.Builder, n *node) {
	if n == nil {
		return
	}
	if n.isLeaf() {
		b.WriteString(n.text)
		return
	}
	writeString(b, n.left)
	writeString(b, n.right)
}

// IsCharBoundary reports whether at falls on a UTF-8 rune boundary (or is
// 0 or Len(), both always boundaries). false for anything out of range.
func (r Rope) IsCharBoundary(at int) bool {
	if at < 0 || at > r.Len() {
		return false
	}
	if at == 0 || at == r.Len() {
		return true
	}
	return utf8.RuneStart(byteAt(r.root, at))
}

func byteAt(n *node, i int) byte {
	for {
		if n.isLeaf() {
			return n.text[i]
		}
		if i < n.weight {
			n = n.left
		} else {
			i -= n.weight
			n = n.right
		}
	}
}

func (r Rope) validate(at int) error {
	if at < 0 || at > r.Len() {
		return ErrOutOfBounds
	}
	if !r.IsCharBoundary(at) {
		return ErrNotCharBoundary
	}
	return nil
}

// Insert returns a new Rope with s inserted at byte offset at.
func (r Rope) Insert(at int, s string) (Rope, error) {
	if err := r.validate(at); err != nil {
		return r, err
	}
	if s == "" {
		return r, nil
	}
	left, right := split(r.root, at)
	return Rope{root: concat(concat(left, build(s)), right)}, nil
}

// Delete returns a new Rope with [start, end) removed.
func (r Rope) Delete(start, end int) (Rope, error) {
	if start > end {
		return r, ErrInvertedRange
	}
	if err := r.validate(start); err != nil {
		return r, err
	}
	if err := r.validate(end); err != nil {
		return r, err
	}
	if start == end {
		return r, nil
	}
	left, mid := split(r.root, start)
	_, right := split(mid, end-start)
	return Rope{root: concat(left, right)}, nil
}

// Slice returns the substring [start, end) without building a new Rope —
// a plain read, not an edit.
func (r Rope) Slice(start, end int) (string, error) {
	if start > end {
		return "", ErrInvertedRange
	}
	if err := r.validate(start); err != nil {
		return "", err
	}
	if err := r.validate(end); err != nil {
		return "", err
	}
	var b strings.Builder
	b.Grow(end - start)
	writeSlice(&b, r.root, start, end)
	return b.String(), nil
}

func writeSlice(b *strings.Builder, n *node, start, end int) {
	if n == nil || start >= end {
		return
	}
	if n.isLeaf() {
		b.WriteString(n.text[start:end])
		return
	}
	if start < n.weight {
		writeSlice(b, n.left, start, min(end, n.weight))
	}
	if end > n.weight {
		writeSlice(b, n.right, max(start-n.weight, 0), end-n.weight)
	}
}

// split divides n at byte offset at into (before, at-or-after), both
// possibly nil (an empty side). O(depth) — no rebalancing needed here;
// concat is where the tree's shape is kept in check.
func split(n *node, at int) (*node, *node) {
	if n == nil {
		return nil, nil
	}
	if at == 0 {
		return nil, n
	}
	if at == n.length {
		return n, nil
	}
	if n.isLeaf() {
		return newLeaf(n.text[:at]), newLeaf(n.text[at:])
	}
	if at < n.weight {
		l, r := split(n.left, at)
		return l, concat(r, n.right)
	}
	l, r := split(n.right, at-n.weight)
	return concat(n.left, l), r
}

// concat joins a and b, rebuilding into a balanced tree when the result's
// depth would exceed what's reasonable for its size — a scapegoat-tree
// style amortized rebalance: cheap on every call except the rare ones
// where it rebuilds, and those become rarer as the rope grows (Skiena
// §3, on amortized analysis of exactly this kind of "occasional expensive
// rebuild" scheme).
func concat(a, b *node) *node {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	n := newBranch(a, b)
	if n.depth > rebalanceThreshold(n.length) {
		return rebuildBalanced(n)
	}
	return n
}

// rebalanceThreshold is roughly 2*log2(leaf count) — generous enough that
// normal editing (sequential typing, scattered small edits) almost never
// triggers a rebuild, tight enough that a pathological sequence can't
// degrade split/concat to O(n).
func rebalanceThreshold(length int) int {
	leaves := length/maxLeafBytes + 1
	return 2*bits.Len(uint(leaves)) + 2
}

func rebuildBalanced(n *node) *node {
	leaves := make([]*node, 0, n.length/maxLeafBytes+1)
	collectLeaves(n, &leaves)
	return buildBalancedFromLeaves(leaves)
}

func collectLeaves(n *node, out *[]*node) {
	if n == nil {
		return
	}
	if n.isLeaf() {
		*out = append(*out, n)
		return
	}
	collectLeaves(n.left, out)
	collectLeaves(n.right, out)
}

func buildBalancedFromLeaves(leaves []*node) *node {
	if len(leaves) == 1 {
		return leaves[0]
	}
	mid := len(leaves) / 2
	return newBranch(buildBalancedFromLeaves(leaves[:mid]), buildBalancedFromLeaves(leaves[mid:]))
}

// Depth is the tree's height — exported only so tests (and, later,
// metrics) can confirm rebalancing is actually keeping it near log2(Len()),
// not a property callers should otherwise depend on.
func (r Rope) Depth() int {
	if r.root == nil {
		return 0
	}
	return r.root.depth
}
