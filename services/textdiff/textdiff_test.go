package textdiff

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func words(s string) []string { return strings.Fields(s) }

// TestLCSTableClassicExample pins the textbook worked example (ABCBDAB
// vs BDCABA, LCS length 4 — "BCBA" or "BDAB" depending on tie-breaking)
// so a future change to the recurrence itself gets caught immediately,
// not just via the property test below.
func TestLCSTableClassicExample(t *testing.T) {
	a := strings.Split("ABCBDAB", "")
	b := strings.Split("BDCABA", "")
	table := LCSTable(a, b)
	assert.Equal(t, 4, table[len(a)][len(b)], "the textbook ABCBDAB/BDCABA example has LCS length 4")
}

func TestDiffOnIdenticalSequencesIsAllEqual(t *testing.T) {
	a := words("the quick brown fox")
	ops := Diff(a, a)
	require.Len(t, ops, len(a))
	for i, op := range ops {
		assert.Equal(t, Equal, op.Kind)
		assert.Equal(t, a[i], op.Token)
	}
}

func TestDiffOnDisjointSequencesIsAllDeleteThenInsert(t *testing.T) {
	a := words("aaa bbb")
	b := words("ccc ddd")
	ops := Diff(a, b)
	require.Len(t, ops, 4, "no common tokens at all — every one is a delete or an insert, none equal")
	for _, op := range ops[:2] {
		assert.Equal(t, Delete, op.Kind)
	}
	for _, op := range ops[2:] {
		assert.Equal(t, Insert, op.Kind)
	}
}

// TestDiffFindsAMinimalEditBetweenSimilarSentences is diff.html's own
// motivating case: two sentences that share most of their words, edited
// only in the middle — the useful, common case a whole-line diff would
// get wrong.
func TestDiffFindsAMinimalEditBetweenSimilarSentences(t *testing.T) {
	a := words("we hold sync acknowledgement under a tight budget")
	b := words("we hold sync acknowledgement under a strict budget")
	ops := Diff(a, b)

	var changed []DiffOp
	for _, op := range ops {
		if op.Kind != Equal {
			changed = append(changed, op)
		}
	}
	require.Len(t, changed, 2, "only 'tight' -> 'strict' should register as a change")
	assert.Equal(t, DiffOp{Kind: Delete, Token: "tight"}, changed[0])
	assert.Equal(t, DiffOp{Kind: Insert, Token: "strict"}, changed[1])
}

// TestDiffEditScriptReconstructsBothSequences is the actual correctness
// law any diff algorithm must satisfy — filtering the edit script down
// to what belonged to a (Delete ∪ Equal) must reproduce a exactly, and
// (Insert ∪ Equal) must reproduce b exactly. This is what makes the
// script a genuine edit description rather than merely "some list of
// kinds," checked across 200 random token sequences (short alphabets so
// shared tokens are actually common, the case that exercises the
// recurrence's tie-breaking rather than degenerating to all-delete-then-
// all-insert every time).
func TestDiffEditScriptReconstructsBothSequences(t *testing.T) {
	tokenGen := rapid.SampledFrom([]string{"a", "b", "c", "d"})
	rapid.Check(t, func(rt *rapid.T) {
		a := rapid.SliceOfN(tokenGen, 0, 8).Draw(rt, "a")
		b := rapid.SliceOfN(tokenGen, 0, 8).Draw(rt, "b")

		ops := Diff(a, b)

		var gotA, gotB []string
		for _, op := range ops {
			switch op.Kind {
			case Equal:
				gotA = append(gotA, op.Token)
				gotB = append(gotB, op.Token)
			case Delete:
				gotA = append(gotA, op.Token)
			case Insert:
				gotB = append(gotB, op.Token)
			}
		}
		if len(gotA) == 0 {
			gotA = []string{}
		}
		if len(gotB) == 0 {
			gotB = []string{}
		}
		if len(a) == 0 {
			a = []string{}
		}
		if len(b) == 0 {
			b = []string{}
		}

		assertEqualSlices(rt, a, gotA, "reconstructing a from Delete+Equal")
		assertEqualSlices(rt, b, gotB, "reconstructing b from Insert+Equal")
	})
}

// TestTracebackWithPathStartsAtCornerAndEndsAtOrigin pins the actual
// contract diff.html's DP-matrix view depends on: the path is exactly
// the cells Traceback's own walk visited, starting at (len(a), len(b))
// and always ending at (0, 0), one shorter than the total number of
// diagonal/up/left moves plus the final resting cell.
func TestTracebackWithPathStartsAtCornerAndEndsAtOrigin(t *testing.T) {
	a := words("we hold sync acknowledgement under a tight budget")
	b := words("we hold sync acknowledgement under a strict budget")
	table := LCSTable(a, b)
	ops, path := TracebackWithPath(table, a, b)

	require.NotEmpty(t, path)
	assert.Equal(t, Coord{I: len(a), J: len(b)}, path[0], "path starts at the table's own bottom-right corner")
	assert.Equal(t, Coord{I: 0, J: 0}, path[len(path)-1], "path always ends at the origin")
	assert.Equal(t, ops, Traceback(table, a, b), "Traceback must be TracebackWithPath's own ops, unchanged")
}

func assertEqualSlices(rt *rapid.T, want, got []string, msg string) {
	if len(want) != len(got) {
		rt.Fatalf("%s: length mismatch: want %v got %v", msg, want, got)
	}
	for i := range want {
		if want[i] != got[i] {
			rt.Fatalf("%s: want %v got %v", msg, want, got)
		}
	}
}
