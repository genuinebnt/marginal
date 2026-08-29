package mdc_test

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/mdc"
)

// ── the lexer ──────────────────────────────────────────────────────────────

// L1 — tokens tile the source exactly: no gap, no overlap, nothing lost.
// Without it "bytes read" on § 11 is an estimate, and an offset anywhere
// downstream is one byte short cumulatively.
func TestTokensTileTheSourceExactly(t *testing.T) {
	for _, src := range []string{
		"# One\n\nSome prose.\n",
		"a\r\nb\r\n",
		"```go\nx := 1\n```\n",
		"- one\n  - nested\n- two",
		"café — dash\n",
		"",
		"\n\n\n",
		"no trailing newline",
	} {
		toks := mdc.Lex(src)
		at := 0
		for i, tk := range toks {
			assert.Equal(t, at, tk.Start, "token %d starts where the last ended (%q)", i, src)
			assert.GreaterOrEqual(t, tk.End, tk.Start)
			at = tk.End
		}
		assert.Equal(t, len(src), at, "tokens must cover the whole source (%q)", src)
	}
}

func TestHeadingLevelFourIsNotAHeading(t *testing.T) {
	// Three levels, because the outline rail indents by level and four stop
	// being legible. `#### x` must stay a paragraph WITH its hashes — eating
	// them would lose text the author typed.
	toks := mdc.Lex("#### deep\n")
	require.Len(t, toks, 1)
	assert.Equal(t, mdc.TokParagraph, toks[0].Kind)
	assert.Equal(t, "#### deep", toks[0].Text)
}

func TestAnUnterminatedFenceRunsToTheEndAndReports(t *testing.T) {
	// The hardest lexer case: a construct that never closes. It must not
	// reset (that loses the fence), must not hang, and must say so.
	r := mdc.Compile("```rust\nfn main() {}\nstill inside\n")
	require.Len(t, r.Tree.Blocks, 1)
	assert.Equal(t, "code_block", r.Tree.Blocks[0].Kind.Tag)
	assert.Contains(t, r.Tree.Blocks[0].Content.Text, "still inside")
	require.NotEmpty(t, r.Diagnostics)
	assert.Contains(t, r.Diagnostics[0].Message, "never closed")
	assert.True(t, r.Holds, "a malformed input still round-trips")
}

// ── inline ─────────────────────────────────────────────────────────────────

func TestInlineMarksLandOnTheStrippedText(t *testing.T) {
	c := mdc.ParseInline("a **bold** and *italic* end")
	assert.Equal(t, "a bold and italic end", c.Text)
	require.Len(t, c.Marks, 2)
	assert.Equal(t, "bold", c.Text[c.Marks[0].Start:c.Marks[0].End])
	assert.Equal(t, "italic", c.Text[c.Marks[1].Start:c.Marks[1].End])
}

func TestAnUnmatchedMarkerStaysLiteral(t *testing.T) {
	// I3. A parser that swallowed the rest of the line looking for a close
	// would eat the paragraph — the most visible inline failure there is.
	c := mdc.ParseInline("2 * 3 = 6")
	assert.Equal(t, "2 * 3 = 6", c.Text)
	assert.Empty(t, c.Marks)
}

func TestPageLinksBeatOrdinaryLinks(t *testing.T) {
	// "[[" is a longer, more specific opener than "[", and checking the
	// short one first turns every page link into a malformed link.
	c := mdc.ParseInline("see [[Block model]] and [docs](https://x.dev)")
	assert.Equal(t, "see Block model and docs", c.Text)
	require.Len(t, c.Marks, 2)
	assert.Equal(t, mdc.PageLink, c.Marks[0].Kind.Tag)
	assert.Equal(t, "Block model", c.Marks[0].Kind.Page)
	assert.Equal(t, mdc.Link, c.Marks[1].Kind.Tag)
	assert.Equal(t, "https://x.dev", c.Marks[1].Kind.URL)
}

func TestEveryMarkOffsetIsOnACharBoundary(t *testing.T) {
	// I2. A mark ending mid-codepoint is a rendering crash, not a wrong
	// colour — so it is checked over multi-byte input specifically.
	c := mdc.ParseInline("café **naïve** — こんにちは *x*")
	for _, m := range c.Marks {
		assert.True(t, isBoundary(c.Text, m.Start), "start %d", m.Start)
		assert.True(t, isBoundary(c.Text, m.End), "end %d", m.End)
	}
}

func isBoundary(s string, i int) bool {
	if i == 0 || i == len(s) {
		return true
	}
	return s[i]&0xC0 != 0x80
}

// ── the grammar ────────────────────────────────────────────────────────────

func TestAListHoldsOnlyItems(t *testing.T) {
	r := mdc.Compile("- one\n- two\n")
	list := r.Tree.Blocks[0]
	assert.Equal(t, "list", list.Kind.Tag)
	for _, b := range r.Tree.Blocks[1:] {
		if b.Parent == list.ID {
			assert.Equal(t, "list_item", b.Kind.Tag, "a list child must be an item")
		}
	}
}

func TestNestingProducesAListInsideAnItemNotAnItemInsideAnItem(t *testing.T) {
	// THE HARDEST TEST HERE. The wrong shape renders identically at one level
	// of depth and only breaks at two, so it survives every casual check.
	r := mdc.Compile("- one\n  - deep\n- two\n")
	byID := map[string]mdc.Block{}
	for _, b := range r.Tree.Blocks {
		byID[b.ID] = b
	}
	var nestedItem mdc.Block
	for _, b := range r.Tree.Blocks {
		if b.Kind.Tag == "list_item" && b.Content.Text == "deep" {
			nestedItem = b
		}
	}
	require.NotEmpty(t, nestedItem.ID, "the nested item exists")
	parent := byID[nestedItem.Parent]
	assert.Equal(t, "list", parent.Kind.Tag, "a nested item's parent is a LIST")
	grand := byID[parent.Parent]
	assert.Equal(t, "list_item", grand.Kind.Tag, "and that list's parent is the item above it")
	assert.Equal(t, "one", grand.Content.Text)
}

func TestAQuoteKeepsItsProseInAChild(t *testing.T) {
	// Containers hold text in children (RFC-001). The reader dropped every
	// container's text for exactly as long as this was not true somewhere.
	r := mdc.Compile("> quoted line\n> and more\n")
	require.Len(t, r.Tree.Blocks, 2)
	assert.Equal(t, "quote", r.Tree.Blocks[0].Kind.Tag)
	assert.Equal(t, "", r.Tree.Blocks[0].Content.Text)
	assert.Equal(t, r.Tree.Blocks[0].ID, r.Tree.Blocks[1].Parent)
	assert.Equal(t, "quoted line and more", r.Tree.Blocks[1].Content.Text)
}

func TestCodeCarriesNoMarks(t *testing.T) {
	r := mdc.Compile("```\na **not bold** b\n```\n")
	require.Len(t, r.Tree.Blocks, 1)
	assert.Empty(t, r.Tree.Blocks[0].Content.Marks)
	assert.Contains(t, r.Tree.Blocks[0].Content.Text, "**not bold**")
}

func TestHardWrappedProseIsOneParagraph(t *testing.T) {
	// Treating a source line break as a paragraph break is the single most
	// common way an importer mangles prose.
	r := mdc.Compile("one line\nand its continuation\n\nsecond para\n")
	assert.Equal(t, 2, len(r.Tree.Blocks))
	assert.Equal(t, "one line and its continuation", r.Tree.Blocks[0].Content.Text)
}

// ── the property § 11 exists for ───────────────────────────────────────────

func TestReplayingTheOpsEqualsTheTreeTheyCameFrom(t *testing.T) {
	for _, src := range corpus() {
		r := mdc.Compile(src)
		assert.True(t, r.Holds, "input %q: %s", short(src), r.Mismatch)
	}
}

// The property test proper: random documents, every one round-tripped. This
// is the one that catches an emission order where a child precedes its
// parent, which no hand-written example reliably produces.
func TestReplayEqualityOverRandomisedInputs(t *testing.T) {
	rng := rand.New(rand.NewSource(0x5EED))
	for i := 0; i < 3000; i++ {
		src := randomDoc(rng)
		r := mdc.Compile(src)
		if !r.Holds {
			t.Fatalf("round trip failed on input %d:\n%s\n\nmismatch: %s", i, src, r.Mismatch)
		}
	}
}

func TestEmissionOrderAppliesCleanlyToAnEmptyTree(t *testing.T) {
	// E1, checked directly rather than through Holds: Replay errors when an
	// op names a parent or sibling that does not exist yet.
	for _, src := range corpus() {
		_, err := mdc.Replay(mdc.Emit(mdc.Compile(src).Tree))
		require.NoError(t, err, "input %q", short(src))
	}
}

func TestCompileIsDeterministic(t *testing.T) {
	// Same input, same ids, same everything — a lab screen whose ids churn on
	// every keystroke re-renders everything on every keystroke.
	src := "# Title\n\n- a\n  - b\n\n> q\n"
	a, b := mdc.Compile(src), mdc.Compile(src)
	assert.Equal(t, a.Tree, b.Tree)
	assert.Equal(t, a.Ops, b.Ops)
}

func TestCharsAndBytesDivergeAndTheDivergenceIsNamed(t *testing.T) {
	r := mdc.Compile("café — こんにちは")
	assert.Less(t, r.Stats.Chars, r.Stats.Bytes)
	assert.NotEmpty(t, r.Stats.Divergences)
}

func TestEmptyInputIsAnEmptyDocumentNotAnError(t *testing.T) {
	r := mdc.Compile("")
	assert.Empty(t, r.Tree.Blocks)
	assert.Empty(t, r.Ops)
	assert.True(t, r.Holds, "nothing round-trips to nothing")
}

// ── fixtures ───────────────────────────────────────────────────────────────

func corpus() []string {
	return []string{
		"",
		"plain",
		"# H1\n## H2\n### H3\n",
		"para one\n\npara two\n",
		"- a\n- b\n",
		"1. first\n2. second\n",
		"- [ ] todo\n- [x] done\n",
		"- a\n  - b\n    - c\n- d\n",
		"> quote\n",
		"---\n",
		"```go\nfmt.Println()\n```\n",
		"```\nunclosed\n",
		"**bold** *it* ~~s~~ `c` [l](u) [[P]]\n",
		"café — naïve\n",
		"# H\n- a\n  - b\n> q\n```rs\nx\n```\n---\ntail\n",
	}
}

func randomDoc(rng *rand.Rand) string {
	lines := []string{}
	n := rng.Intn(14)
	for i := 0; i < n; i++ {
		switch rng.Intn(9) {
		case 0:
			lines = append(lines, fmt.Sprintf("%s heading %d", strings.Repeat("#", 1+rng.Intn(4)), i))
		case 1:
			lines = append(lines, fmt.Sprintf("prose %d with **bold** and [[Link %d]]", i, i))
		case 2:
			lines = append(lines, fmt.Sprintf("%s- item %d", strings.Repeat("  ", rng.Intn(3)), i))
		case 3:
			lines = append(lines, fmt.Sprintf("%d. ordered %d", i+1, i))
		case 4:
			lines = append(lines, fmt.Sprintf("- [%s] todo %d", map[bool]string{true: "x", false: " "}[rng.Intn(2) == 0], i))
		case 5:
			lines = append(lines, "> quoted "+fmt.Sprint(i))
		case 6:
			lines = append(lines, "```", "code "+fmt.Sprint(i), "```")
		case 7:
			lines = append(lines, "---")
		default:
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}

func short(s string) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) > 40 {
		return s[:40] + "…"
	}
	return s
}
