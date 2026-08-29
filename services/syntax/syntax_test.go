package syntax_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/syntax"
)

func joined(toks []syntax.Token) string {
	var b strings.Builder
	for _, t := range toks {
		b.WriteString(t.Text)
	}
	return b.String()
}

func kindContaining(toks []syntax.Token, text string) syntax.Kind {
	for _, t := range toks {
		if strings.Contains(t.Text, text) {
			return t.Kind
		}
	}
	return ""
}

func kindOf(toks []syntax.Token, text string) syntax.Kind {
	for _, t := range toks {
		if t.Text == text {
			return t.Kind
		}
	}
	return ""
}

// S1 — THE invariant. Concatenating every token reproduces the input exactly.
// A highlighter that loses or duplicates a character has corrupted the code on
// screen, which is far worse than colouring it wrong.
func TestTokensAlwaysReproduceTheInput(t *testing.T) {
	cases := []struct{ lang, src string }{
		{"go", "func main() {\n\tfmt.Println(\"hi\") // greet\n}"},
		{"rust", "let x: Vec<u8> = vec![1, 2, 3]; /* note */"},
		{"ts", "const a = `tpl ${x}`; // c"},
		{"sql", "SELECT id FROM docs.pages WHERE deleted_at IS NULL; -- all"},
		{"python", "def f(x):\n    return x ** 2  # square"},
		{"bash", "for f in *.go; do echo \"$f\"; done"},
		{"json", "{\"a\": [1, true, null]}"},
		{"", "no language at all — 3 + 4 // still a comment"},
		{"go", ""},
		{"go", "\n\n\n"},
		{"go", "héllo := \"wörld\" // ünicode"},
	}
	for _, c := range cases {
		assert.Equal(t, c.src, joined(syntax.Highlight(c.lang, c.src)),
			"lang=%q must round-trip", c.lang)
	}
}

func TestAnUnterminatedStringStopsAtTheNewline(t *testing.T) {
	// The single most visible highlighter failure: one stray quote turning the
	// whole rest of the block into a string. It must end at the line.
	toks := syntax.Highlight("go", "a := \"oops\nb := 1")
	assert.Equal(t, "a := \"oops\nb := 1", joined(toks))
	assert.Equal(t, syntax.Number, kindOf(toks, "1"), "the next line still lexes normally")
}

func TestAnUnterminatedBlockCommentDoesNotHang(t *testing.T) {
	// A lexer that cannot advance past a malformed token hangs — in
	// production, on someone's pasted code. It runs to end of input instead.
	src := "/* never closed\nstill inside"
	toks := syntax.Highlight("go", src)
	require.Len(t, toks, 1)
	assert.Equal(t, syntax.Comment, toks[0].Kind)
	assert.Equal(t, src, toks[0].Text)
}

func TestARawStringIgnoresBackslashEscapes(t *testing.T) {
	// Go's backtick string has no escapes. A lexer that honours \" inside one
	// swallows the closing quote and then the rest of the file.
	toks := syntax.Highlight("go", "s := `a\\` + b")
	assert.Equal(t, syntax.String, kindOf(toks, "`a\\`"))
	assert.Equal(t, "s := `a\\` + b", joined(toks))
}

func TestKeywordsBeatTheCallHeuristic(t *testing.T) {
	// `if (x)` in a C-like language is an identifier followed by "(" — the
	// call heuristic would colour it as a function. Keywords are checked
	// first precisely because of this case.
	toks := syntax.Highlight("ts", "if (ready) render(x);")
	assert.Equal(t, syntax.Keyword, kindOf(toks, "if"))
	assert.Equal(t, syntax.Func, kindOf(toks, "render"))
}

func TestAnIdentifierContainingDigitsIsNotANumber(t *testing.T) {
	// Note this also pins the merging rule: `utf8Len` and the space after it
	// are both Plain and arrive as ONE token, so the assertion is about which
	// token contains it rather than about an exact match.
	toks := syntax.Highlight("go", "utf8Len := 3")
	assert.Equal(t, syntax.Plain, kindContaining(toks, "utf8Len"))
	assert.Equal(t, syntax.Number, kindOf(toks, "3"))
}

func TestAnUnknownLanguageStillFindsStringsNumbersAndComments(t *testing.T) {
	// Not an error and not plain text: those three rules are right in almost
	// every language, and a code block with none of them coloured looks broken
	// rather than unsupported.
	toks := syntax.Highlight("brainfuck", "x = \"s\" + 42 // note")
	assert.Equal(t, syntax.String, kindOf(toks, "\"s\""))
	assert.Equal(t, syntax.Number, kindOf(toks, "42"))
	assert.Equal(t, syntax.Comment, kindOf(toks, "// note"))
}

func TestAdjacentRunsOfOneKindAreMerged(t *testing.T) {
	// Without merging, a line of punctuation is one DOM node per character.
	toks := syntax.Highlight("go", "a := b[c][d]")
	for _, tk := range toks {
		if tk.Kind == syntax.Punct {
			assert.NotEqual(t, 1, 0) // presence is enough; the assertion is below
		}
	}
	// "][" between the two indexes must be ONE punct token, not two.
	assert.Contains(t, toks, syntax.Token{Kind: syntax.Punct, Text: "]["})
}

func TestAliasesShareOneLexer(t *testing.T) {
	for _, alias := range []string{"js", "javascript", "typescript", "tsx"} {
		toks := syntax.Highlight(alias, "const x = 1")
		assert.Equal(t, syntax.Keyword, kindOf(toks, "const"), "alias %q", alias)
	}
}

func TestLanguagesIsSortedAndIncludesTheAliases(t *testing.T) {
	names := syntax.Languages()
	require.NotEmpty(t, names)
	assert.IsIncreasing(t, names)
	assert.Contains(t, names, "go")
	assert.Contains(t, names, "javascript")
}
