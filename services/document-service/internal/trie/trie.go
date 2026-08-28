// Package trie is search.html's other real DSA claim, and RELEASES.md's
// own "[[link]]/command autocomplete via a trie while typing" — a plain
// prefix tree over page titles, pure and dependency-free (graphalgo's
// own discipline, and internal/bktree's). Compiled to wasm
// (document-service/cmd/triewasm) because `[[` autocomplete needs
// interactive, per-keystroke response — the same reasoning cmd/graphwasm
// and cmd/diffwasm's own doc comments give for anything that has to run
// against live client state.
//
// This repo's own re-cut scope carries the trie forward from the
// original Rust-track ROADMAP.md Phase 7 but not its Levenshtein-
// automaton sibling (that pairs with a trie for FUZZY prefix search over
// a large vocabulary — full-text search, internal/search, already
// covers fuzzy matching over *content*; PrefixSearch below is exact-
// prefix only, which is what typing "[[Arch" to find "Architecture"
// actually needs).
package trie

// node is one trie node — Children keyed by the next rune, IsTitle set
// exactly on the nodes where a full title ends (so "Arch" being a prefix
// of "Architecture" doesn't itself count as a match unless "Arch" is
// also, separately, a real title).
type node struct {
	children map[rune]*node
	titles   []string // every title that ends exactly here (docs.pages' own title uniqueness is NOT enforced, so more than one page can share a title)
}

func newNode() *node { return &node{children: make(map[rune]*node)} }

// Trie is a prefix tree over page titles. Zero value is an empty trie,
// ready to Insert into.
type Trie struct {
	root *node
}

// Insert adds title to the trie — O(len(title)) rune comparisons,
// walking one child edge per rune, creating any missing node along the
// way. Walked by its lowercased form (PrefixSearch's own doc comment
// explains why), but the original casing is what's stored and returned.
func (t *Trie) Insert(title string) {
	if t.root == nil {
		t.root = newNode()
	}
	n := t.root
	for _, r := range []rune(lower(title)) {
		child, ok := n.children[r]
		if !ok {
			child = newNode()
			n.children[r] = child
		}
		n = child
	}
	n.titles = append(n.titles, title)
}

// PrefixSearch returns every title starting with prefix, walked
// straight down the trie by prefix (O(len(prefix))) and then collected
// via one DFS over the subtree the prefix's own node roots — the whole
// point of a trie over a linear scan: the cost of finding matches is
// proportional to the prefix typed plus the matches found, never to how
// many OTHER titles exist in the workspace that don't share it. Case-
// insensitive (matching page titles' own existing lower(title) index —
// DATA_MODEL.md) and returns nil, not an error, for a prefix nothing
// starts with — an empty autocomplete dropdown, not a failure.
func (t *Trie) PrefixSearch(prefix string) []string {
	if t.root == nil {
		return nil
	}
	n := t.root
	for _, r := range []rune(lower(prefix)) {
		child, ok := n.children[r]
		if !ok {
			return nil
		}
		n = child
	}

	var out []string
	var walk func(*node)
	walk = func(n *node) {
		out = append(out, n.titles...)
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(n)
	return out
}

// lower is a tiny, dependency-free ASCII+common-case lowercaser — this
// package stays free of strings/unicode imports the same way
// graphalgo/bktree do, and page titles are typed text, not a locale-
// sensitive sorting problem.
func lower(s string) string {
	runes := []rune(s)
	for i, r := range runes {
		if r >= 'A' && r <= 'Z' {
			runes[i] = r + ('a' - 'A')
		}
	}
	return string(runes)
}
