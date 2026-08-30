package netsim_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/netsim"
)

func scenario(transform bool, edits ...netsim.Edit) netsim.Scenario {
	return netsim.Scenario{
		Wire:      netsim.Wire{RTTMs: 180, LossPct: 0, JitterMs: 20, Seed: 7},
		Transform: transform,
		Initial:   "A rope is the wrong primitive.",
		Edits:     edits,
	}
}

func ins(at int, actor string, pos int, text string) netsim.Edit {
	return netsim.Edit{At: at, Actor: actor, Kind: netsim.Insert, Pos: pos, Text: text}
}
func del(at int, actor string, pos, n int) netsim.Edit {
	return netsim.Edit{At: at, Actor: actor, Kind: netsim.Delete, Pos: pos, Len: n}
}

// ── transform ────────────────────────────────────────────────────

// The property the whole page is about: two concurrent edits, applied
// in either order, must produce the same document.
func TestTransformConverges(t *testing.T) {
	base := "abcdef"
	cases := []struct{ a, b netsim.Op }{
		{netsim.Op{Actor: "x", Kind: netsim.Insert, Pos: 2, Text: "XX"},
			netsim.Op{Actor: "y", Kind: netsim.Insert, Pos: 4, Text: "YY"}},
		{netsim.Op{Actor: "x", Kind: netsim.Insert, Pos: 3, Text: "X"},
			netsim.Op{Actor: "y", Kind: netsim.Delete, Pos: 1, Len: 3}},
		{netsim.Op{Actor: "x", Kind: netsim.Delete, Pos: 0, Len: 3},
			netsim.Op{Actor: "y", Kind: netsim.Delete, Pos: 2, Len: 3}},
		{netsim.Op{Actor: "x", Kind: netsim.Delete, Pos: 1, Len: 2},
			netsim.Op{Actor: "y", Kind: netsim.Insert, Pos: 2, Text: "Z"}},
	}
	for i, c := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			ab := netsim.Transform(c.b, c.a).Apply(c.a.Apply(base))
			ba := netsim.Transform(c.a, c.b).Apply(c.b.Apply(base))
			assert.Equal(t, ab, ba, "order changed the document — TP1 is broken")
		})
	}
}

// Two inserts at the same position are the case where a per-replica
// tie-break silently produces two different documents. The tie-break
// has to be something both sides compute the same way.
func TestConcurrentInsertsAtTheSamePositionStillConverge(t *testing.T) {
	base := "ab"
	a := netsim.Op{Actor: "ada", Kind: netsim.Insert, Pos: 1, Text: "A"}
	b := netsim.Op{Actor: "bo", Kind: netsim.Insert, Pos: 1, Text: "B"}
	assert.Equal(t,
		netsim.Transform(b, a).Apply(a.Apply(base)),
		netsim.Transform(a, b).Apply(b.Apply(base)))
}

func TestInvertUndoesTheOp(t *testing.T) {
	base := "A rope is wrong"
	for _, op := range []netsim.Op{
		{Kind: netsim.Insert, Pos: 2, Text: "very "},
		{Kind: netsim.Delete, Pos: 2, Len: 4},
		{Kind: netsim.Delete, Pos: 0, Len: 15},
	} {
		after := op.Apply(base)
		assert.Equal(t, base, op.Invert(base).Apply(after), "%v did not invert", op)
	}
}

// ── the simulation ───────────────────────────────────────────────

func TestBothReplicasConvergeOnTheServerText(t *testing.T) {
	r := netsim.Run(scenario(true,
		ins(0, "you", 10, "quite "),
		ins(5, "ada", 22, "wrong "),
		del(30, "you", 0, 2),
	))
	require.Len(t, r.Replicas, 2)
	for _, rep := range r.Replicas {
		assert.Equal(t, r.ServerText, rep.Text, "%s diverged", rep.Actor)
	}
	assert.True(t, r.Converged)
}

// RFC-002's law, which the screen re-checks rather than asserts.
func TestReplayFromEmptyMatchesTheIncrementalView(t *testing.T) {
	r := netsim.Run(scenario(true,
		ins(0, "you", 2, "long "), ins(3, "ada", 12, "not "), del(9, "you", 0, 1),
	))
	assert.True(t, r.ReplayMatches, "replay %q != server %q", r.ReplayText, r.ServerText)
}

// Prediction is the point of local echo: the author's own text moves
// on the tick they typed, a round trip before the server has heard
// of it. Checked by running with a wire so slow the op cannot
// possibly have been confirmed, and asserting the text moved anyway.
func TestPredictionIsImmediate(t *testing.T) {
	s := scenario(true, ins(0, "you", 0, "Z"))
	s.Wire.RTTMs = 4000
	r := netsim.Run(s)
	require.Len(t, r.Replicas, 1)
	assert.Equal(t, 1, r.Replicas[0].Predicted)
	assert.True(t, strings.HasPrefix(r.Replicas[0].Text, "Z"),
		"the author waited for the server: %q", r.Replicas[0].Text)
}

// A lost packet must cost a retransmit, never a keystroke.
func TestALostOpIsResentNotDropped(t *testing.T) {
	s := scenario(true, ins(0, "you", 0, "Z"), ins(4, "ada", 5, "Y"))
	s.Wire.LossPct = 60
	r := netsim.Run(s)
	assert.Positive(t, r.Retransmits, "60%% loss produced no retransmits at all")
	assert.Len(t, r.Log, 2, "an op was lost outright")
	assert.True(t, r.Converged)
}

// The same seed twice is a test; a different answer each run is an
// anecdote. This is what makes the screen's wire controls usable.
func TestARunIsReproducible(t *testing.T) {
	s := scenario(true, ins(0, "you", 3, "aa"), ins(1, "ada", 9, "bb"), del(7, "ada", 0, 3))
	s.Wire.LossPct = 25
	a, b := netsim.Run(s), netsim.Run(s)
	assert.Equal(t, a.ServerText, b.ServerText)
	assert.Equal(t, a.Retransmits, b.Retransmits)
	assert.Equal(t, a.Log, b.Log)
}

// The page's own argument, as a test: with transform off the replicas
// still AGREE — the structural digest is happy — and the document is
// wrong. Two instruments, disagreeing on purpose.
func TestWithoutTransformTheReplicasAgreeOnAWrongDocument(t *testing.T) {
	edits := []netsim.Edit{del(0, "you", 0, 2), ins(40, "ada", 10, "very ")}
	on := netsim.Run(scenario(true, edits...))
	off := netsim.Run(scenario(false, edits...))

	require.True(t, on.Converged)
	assert.NotEqual(t, on.ServerText, off.ServerText,
		"this scenario does not actually need a transform — pick a harder one")
	assert.NotEmpty(t, off.IntentViolations, "the intent ledger flagged nothing")
	assert.Empty(t, on.IntentViolations, "transform is on; there is nothing to flag")
}

// ── merkle ───────────────────────────────────────────────────────

func TestIdenticalTextsAgreeAtTheRoot(t *testing.T) {
	v := netsim.CompareMerkle("the same bytes exactly", "the same bytes exactly")
	assert.True(t, v.Equal)
	assert.Equal(t, 1, v.ComparedNodes, "agreement must cost exactly one hash")
	for _, n := range v.Nodes {
		assert.False(t, n.Divergence)
	}
}

// The point of the tree: finding WHERE, not just whether.
func TestOneChangedByteIsLocatedInLogNComparisons(t *testing.T) {
	left := strings.Repeat("abcdefgh", 8) // 64 bytes, 8 leaves
	right := left[:40] + "Z" + left[41:]
	v := netsim.CompareMerkle(left, right)

	require.False(t, v.Equal)
	assert.Less(t, v.ComparedNodes, len(v.Nodes),
		"it walked the whole tree instead of descending")

	var marked []string
	for _, n := range v.Nodes {
		if n.Divergence {
			marked = append(marked, n.ID)
		}
	}
	assert.Len(t, marked, 1, "there is one edit, so there is one divergence point")
}

func TestDifferentLengthsStillCompare(t *testing.T) {
	v := netsim.CompareMerkle("short", "considerably longer text here")
	assert.False(t, v.Equal)
	assert.NotEmpty(t, v.Nodes)
}

// ── causality ────────────────────────────────────────────────────

// The longest chain is the round trips this session could not have
// avoided. Everything off it happened concurrently.
func TestLongestChainIsTheCriticalPath(t *testing.T) {
	r := netsim.Run(scenario(true,
		ins(0, "you", 0, "a"), ins(400, "you", 1, "b"), ins(800, "you", 2, "c"),
	))
	// Each edit waits a full round trip, so each sees the last.
	assert.Equal(t, 3, r.Causality.LongestChain)
	assert.Zero(t, r.Causality.Concurrent, "nothing here is concurrent")
}

func TestSimultaneousEditsShareALayer(t *testing.T) {
	r := netsim.Run(scenario(true, ins(0, "you", 0, "a"), ins(0, "ada", 5, "b")))
	assert.Positive(t, r.Causality.Concurrent,
		"two edits typed on the same tick are concurrent by definition")
	assert.LessOrEqual(t, r.Causality.LongestChain, 2)
}

// ── lsm ──────────────────────────────────────────────────────────

func TestAnEmptyLogHasAnEmptyMemtableAndNoFiles(t *testing.T) {
	v := netsim.BuildLSM(0)
	require.NotEmpty(t, v.Levels)
	assert.Zero(t, v.Levels[0].Ops)
	assert.Zero(t, v.WriteAmplification)
}

func TestWriteAmplificationRisesWithDepth(t *testing.T) {
	shallow := netsim.BuildLSM(100)
	deep := netsim.BuildLSM(100000)
	assert.GreaterOrEqual(t, shallow.WriteAmplification, 1.0,
		"every op is written at least once")
	assert.Greater(t, deep.WriteAmplification, shallow.WriteAmplification,
		"a deeper tree rewrites more — that is the cost of the shape")
}

// ── the editable script ──────────────────────────────────────────

func TestParseSkipsHalfLinesRatherThanFailing(t *testing.T) {
	edits, skipped := netsim.ParseScenario(strings.Join([]string{
		"# a comment",
		"0, you, insert, 10, quite ",
		"nonsense",
		"5, ada, delete, 2, 4",
		"9, you, insert,",         // half typed
		"12, you, sideways, 1, x", // no such op
		"20, ada, delete, 0, nope",
	}, "\n"))
	require.Len(t, edits, 2)
	assert.Equal(t, "quite ", edits[0].Text, "a trailing space is a character somebody typed")
	assert.Equal(t, 4, edits[1].Len)
	assert.Equal(t, 4, skipped)
}

func TestInsertedTextMayContainCommas(t *testing.T) {
	edits, skipped := netsim.ParseScenario("0, you, insert, 3, one, two, three")
	require.Len(t, edits, 1)
	assert.Zero(t, skipped)
	assert.Equal(t, "one, two, three", edits[0].Text)
}

// Every slice on the wire must be an ARRAY, never null.
//
// encoding/json renders a nil slice as `null`; § 14 reads
// `report.log.length`. An empty script is not an edge case — the
// screen passes through it on every keystroke that clears the
// textarea — and it took the whole page down. Asserted at the
// boundary, because from inside Go a nil slice and an empty one
// behave identically.
func TestNoSliceReachesTheWireAsNull(t *testing.T) {
	for _, name := range []string{"empty script", "one edit"} {
		t.Run(name, func(t *testing.T) {
			s := scenario(true)
			if name == "one edit" {
				s.Edits = []netsim.Edit{ins(0, "you", 0, "Z")}
			}
			data, err := json.Marshal(netsim.Run(s))
			require.NoError(t, err)

			var wire map[string]any
			require.NoError(t, json.Unmarshal(data, &wire))
			for _, key := range []string{
				"replicas", "log", "deliveries", "intent_violations",
			} {
				v, present := wire[key]
				require.True(t, present, "%s is missing entirely", key)
				assert.NotNil(t, v, "%s arrived as null — the screen calls .length on it", key)
			}
			for _, path := range [][2]string{
				{"merkle", "nodes"}, {"causality", "nodes"}, {"lsm", "levels"},
			} {
				sub, ok := wire[path[0]].(map[string]any)
				require.True(t, ok, "%s is not an object", path[0])
				assert.NotNil(t, sub[path[1]], "%s.%s arrived as null", path[0], path[1])
			}
		})
	}
}
