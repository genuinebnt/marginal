// Package textdiff is docs/ui-mockups/diff.html's own algorithm, real:
// "LCS by the full O(n·m) dynamic-programming table" plus "the traceback
// that turns that table into an edit script." Pure functions over
// []string — no I/O, no service dependency, the same
// dependency-free-package discipline graphalgo already set (its own
// second consumer, this repo's proof that top-level extraction pays off:
// document-service's cmd/diffwasm compiles this to wasm because
// diff.html's own "token granularity switching (word ↔ character),
// recomputed live" needs interactive, client-side response to a toggle,
// the same reasoning cmd/graphwasm's own doc comment already gives for
// the force layout and Voronoi/Delaunay).
//
// Tokens are caller-supplied strings, not bytes or runes — the caller
// decides word-granularity (split on whitespace) or character-granularity
// (one token per rune) before calling in; this package only ever
// compares tokens for equality, so it never has to know which.
//
// The O(n·m) memory the DP table costs is deliberate, not an oversight —
// this package's own doc comment states the boundary plainly: fine for a
// paragraph, absurd for a whole document (ROADMAP.md § DP names Myers'
// O(nd) algorithm as the answer at that scale — out of scope here, since
// diff.html only ever diffs one block's own text at a time).
package textdiff

// LCSTable is the classic longest-common-subsequence dynamic-programming
// table: table[i][j] is the LCS length of a[:i] and b[:j]. table has
// (len(a)+1) rows and (len(b)+1) columns — row/column 0 is the "empty
// prefix" base case, LCS length 0 against anything.
func LCSTable(a, b []string) [][]int {
	table := make([][]int, len(a)+1)
	for i := range table {
		table[i] = make([]int, len(b)+1)
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				table[i][j] = table[i-1][j-1] + 1
			} else if table[i-1][j] >= table[i][j-1] {
				table[i][j] = table[i-1][j]
			} else {
				table[i][j] = table[i][j-1]
			}
		}
	}
	return table
}

// Kind is one traced edit-script step's own tag.
type Kind string

const (
	Equal  Kind = "equal"
	Delete Kind = "delete" // present in a, absent from b
	Insert Kind = "insert" // absent from a, present in b
)

// DiffOp is one token's fate in the edit script Traceback produces, in
// document order (an Equal token appears once; a token replaced outright
// appears as an adjacent Delete then Insert pair, never a third "Replace"
// kind — diff.html's own DP matrix visualization only ever colors a cell
// one of these three ways).
type DiffOp struct {
	Kind  Kind   `json:"kind"`
	Token string `json:"token"`
}

// Traceback walks table (LCSTable's own output for the same a, b) from
// (len(a), len(b)) back to (0, 0) and returns the edit script in forward
// (document) order — the standard LCS traceback: a matching token moves
// diagonally (Equal); otherwise the neighbor with the larger LCS value
// is preferred (Delete moves up through a, Insert moves left through
// b). Ties are broken toward Insert *during this backward walk*, which
// — because the walk's own output gets reversed into forward order
// below — is what actually produces the conventional "every Delete
// before its neighboring Insert" reading order a diff view expects
// (`diff -u`'s own convention: what was removed, then what replaced
// it), not the reverse.
func Traceback(table [][]int, a, b []string) []DiffOp {
	ops, _ := TracebackWithPath(table, a, b)
	return ops
}

// Coord is one (i, j) cell of the LCS table — TracebackWithPath's own
// visited-cell trail, diff.html's own "the outlined path is the
// traceback that becomes the edit script," drawn from what the walk
// actually visited rather than re-derived a second time client-side.
type Coord struct {
	I int `json:"i"`
	J int `json:"j"`
}

// TracebackWithPath is Traceback's own body, additionally returning
// every (i, j) cell the backward walk actually visited, oldest-in-the-
// walk first (i.e. starting at (len(a), len(b)), ending at (0, 0)) — the
// exact path diff.html's own DP-matrix view outlines. Traceback is this
// function with the path discarded, not a separate implementation.
func TracebackWithPath(table [][]int, a, b []string) ([]DiffOp, []Coord) {
	i, j := len(a), len(b)
	var reversed []DiffOp
	var path []Coord
	for i > 0 || j > 0 {
		path = append(path, Coord{I: i, J: j})
		switch {
		case i > 0 && j > 0 && a[i-1] == b[j-1]:
			reversed = append(reversed, DiffOp{Kind: Equal, Token: a[i-1]})
			i--
			j--
		case i > 0 && (j == 0 || table[i-1][j] > table[i][j-1]):
			reversed = append(reversed, DiffOp{Kind: Delete, Token: a[i-1]})
			i--
		default:
			reversed = append(reversed, DiffOp{Kind: Insert, Token: b[j-1]})
			j--
		}
	}
	path = append(path, Coord{I: 0, J: 0})

	ops := make([]DiffOp, len(reversed))
	for k, op := range reversed {
		ops[len(reversed)-1-k] = op
	}
	return ops, path
}

// Diff is the whole pipeline — build the table, trace it back — for a
// caller that has no use for the table itself (LCSTable is still
// exported on its own for diff.html's DP-matrix visualization, which
// needs every cell's own computed value, not just the final script).
func Diff(a, b []string) []DiffOp {
	return Traceback(LCSTable(a, b), a, b)
}
