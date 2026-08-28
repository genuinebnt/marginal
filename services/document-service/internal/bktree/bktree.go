// Package bktree is search.html's own "fuzzy title matching is a
// metric-tree search, so a typo still finds the page" — a Burkhard-Keller
// tree over page titles, pure and dependency-free (graphalgo's own
// discipline). ROADMAP.md § Two fuzzy searches deliberately: "BK-tree
// prunes by the triangle inequality in a metric space — a subtree whose
// distance bound excludes the query is skipped." Levenshtein distance is
// the metric this tree is built over; the triangle inequality only holds
// because it IS a true metric (symmetric, zero iff equal, and
// d(x,z) <= d(x,y) + d(y,z)) — that's what makes pruning by distance
// bound sound rather than a heuristic.
//
// This repo's own re-cut scope (docs/planning/RELEASES.md's v2.5.0)
// carries the BK-tree forward from the original Rust-track ROADMAP.md
// Phase 7 but not its Levenshtein-automaton-over-a-trie sibling — that
// one solves the same problem for a large shared-prefix vocabulary
// (every token in the notebook), which full-text search (Postgres FTS)
// already covers at this repo's scope; a modest set of whole page titles
// is exactly the case a BK-tree is good at.
package bktree

// Levenshtein is the classic edit-distance dynamic-programming
// recurrence — insertions, deletions, substitutions, each cost 1 — over
// runes, not bytes (a title is user-authored text, not guaranteed
// ASCII). O(n·m) time and O(min(n,m)) space: only the previous and
// current row are kept, the same "row-by-row over resmaller string"
// space optimization textdiff's own LCSTable deliberately does NOT make
// (that one needs the whole table for diff.html's DP-matrix
// visualization; this one only ever needs the final distance).
func Levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) < len(rb) {
		ra, rb = rb, ra
	}
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	curr := make([]int, len(rb)+1)
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min3(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// node is one BK-tree entry — Word plus a child per distance bucket
// (every word ever inserted at exactly that Levenshtein distance from
// Word lives in children[thatDistance]).
type node struct {
	word     string
	children map[int]*node
}

// Tree is a BK-tree over Levenshtein distance. Zero value is an empty
// tree, ready to Insert into.
type Tree struct {
	root *node
}

// Insert adds word to the tree — O(depth) comparisons, each an O(n·m)
// Levenshtein call, descending one distance-bucket edge per level until
// an empty bucket is found.
func (t *Tree) Insert(word string) {
	if t.root == nil {
		t.root = &node{word: word, children: make(map[int]*node)}
		return
	}
	n := t.root
	for {
		d := Levenshtein(n.word, word)
		if d == 0 {
			return // already present — a BK-tree has no use for exact duplicates
		}
		child, ok := n.children[d]
		if !ok {
			n.children[d] = &node{word: word, children: make(map[int]*node)}
			return
		}
		n = child
	}
}

// Match is one fuzzy hit: the word found and its real Levenshtein
// distance from the query (so a caller can rank "closer" ahead of
// "further," or show why it matched — search.html's own "the snippet
// shows why it matched").
type Match struct {
	Word     string
	Distance int
}

// Query returns every word within maxDistance of query, by the triangle
// inequality: at each node, only children whose bucket distance d
// satisfies |d - distanceToQuery| <= maxDistance can possibly hold a
// match (any word in bucket d is exactly distance d from n.word, and
// d(word, query) >= |d(word, n.word) - d(n.word, query)| by the
// triangle inequality applied twice) — every other subtree is skipped
// without ever computing a single Levenshtein distance inside it, the
// whole point of building this tree instead of scanning every title
// linearly.
func (t *Tree) Query(query string, maxDistance int) []Match {
	if t.root == nil {
		return nil
	}
	var matches []Match
	var walk func(n *node)
	walk = func(n *node) {
		d := Levenshtein(n.word, query)
		if d <= maxDistance {
			matches = append(matches, Match{Word: n.word, Distance: d})
		}
		for bucket, child := range n.children {
			if bucket >= d-maxDistance && bucket <= d+maxDistance {
				walk(child)
			}
		}
	}
	walk(t.root)
	return matches
}
