// Package syntax highlights a code block: source in, a flat run of typed
// tokens out.
//
// WHY THIS IS GO AND NOT A JS LIBRARY. The house rule (CLAUDE.md) is that the
// algorithm lives in Go and TypeScript only draws what Go computed. A
// highlighter is an algorithm — it is a lexer, the same lexer the corpus's own
// "Lexing is a state machine" argues about — and importing one would have put
// the single most-read algorithm in the app on the other side of the boundary
// every other algorithm here respects.
//
// WHAT IT IS NOT. It is not a parser and does not try to be. It has no
// grammar, no scope tracking, and no idea whether an identifier is a type or
// a variable beyond a heuristic. That is a deliberate ceiling: a highlighter
// that is 95% right on nine languages is worth more here than one that is
// 100% right on one, and the failure mode of a lexer-only highlighter is a
// word in the wrong colour, never a crash and never a wrong PROGRAM.
//
// Tokens carry TEXT, not offsets. Offsets across the wasm boundary would be
// byte offsets on this side and UTF-16 indices on the other — the exact
// mismatch documented as an open gap for marks — and a highlighter has no
// reason to inherit it. Concatenating every token's text reproduces the input
// exactly, which is invariant S1 below and the only correctness property that
// really matters.
package syntax

import (
	"sort"
	"strings"
	"unicode"
)

// Kind is what a token IS, semantically — never a colour. The palette belongs
// to the design system (DESIGN_GUIDELINES.md §3), and a lexer that emitted
// "#E8873C" would have hard-coded a stylesheet into an algorithm.
type Kind string

const (
	Plain   Kind = "plain"
	Keyword Kind = "keyword"
	Type    Kind = "type"
	String  Kind = "string"
	Number  Kind = "number"
	Comment Kind = "comment"
	Func    Kind = "func"
	Punct   Kind = "punct"
)

// Token is one run of source with one kind.
type Token struct {
	Kind Kind   `json:"kind"`
	Text string `json:"text"`
}

// Lang describes one language's lexical surface. Everything a language needs
// to differ in is a field here, so adding one is data rather than code.
type Lang struct {
	Keywords    map[string]bool
	Types       map[string]bool
	LineComment []string // "//", "#", "--"
	BlockOpen   string
	BlockClose  string
	// StringQuotes are the delimiters that start a string. Backtick is listed
	// where it is RAW (Go), because a raw string ignores backslash escapes and
	// a lexer that does not know that swallows the rest of the file on the
	// first `\` it meets.
	StringQuotes string
	RawQuotes    string
}

var langs = map[string]Lang{
	"go": {
		Keywords:    set("break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var"),
		Types:       set("bool byte complex64 complex128 error float32 float64 int int8 int16 int32 int64 rune string uint uint8 uint16 uint32 uint64 uintptr any nil true false iota make new len cap append copy delete panic recover"),
		LineComment: []string{"//"},
		BlockOpen:   "/*", BlockClose: "*/",
		StringQuotes: "\"'", RawQuotes: "`",
	},
	"rust": {
		Keywords:    set("as async await break const continue crate dyn else enum extern fn for if impl in let loop match mod move mut pub ref return self Self static struct super trait type unsafe use where while"),
		Types:       set("bool char f32 f64 i8 i16 i32 i64 i128 isize u8 u16 u32 u64 u128 usize str String Vec Option Result Box Rc Arc HashMap BTreeMap Some None Ok Err true false"),
		LineComment: []string{"//"},
		BlockOpen:   "/*", BlockClose: "*/",
		StringQuotes: "\"'",
	},
	"ts": {
		Keywords:    set("as async await break case catch class const continue default delete do else enum export extends finally for from function if implements import in instanceof interface let new of return static super switch this throw try type typeof var void while yield satisfies keyof readonly"),
		Types:       set("any boolean never null number object string symbol undefined unknown true false Array Promise Record Partial Map Set console"),
		LineComment: []string{"//"},
		BlockOpen:   "/*", BlockClose: "*/",
		StringQuotes: "\"'`",
	},
	"python": {
		Keywords:    set("and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield match case"),
		Types:       set("bool bytes dict float int list None set str tuple True False self print len range enumerate"),
		LineComment: []string{"#"},
		StringQuotes: "\"'",
	},
	"sql": {
		Keywords:    set("select from where group by having order limit offset insert into values update set delete create table index view alter drop join left right inner outer on as and or not null distinct returning with union all case when then else end primary key foreign references default constraint check unique generated always stored"),
		Types:       set("int integer bigint smallint text varchar char boolean timestamptz timestamp date time uuid jsonb json numeric decimal real double serial ltree tsvector array"),
		LineComment: []string{"--"},
		BlockOpen:   "/*", BlockClose: "*/",
		StringQuotes: "'\"",
	},
	"bash": {
		Keywords:    set("if then else elif fi for while do done case esac function return export local set unset echo cd exit source"),
		LineComment: []string{"#"},
		StringQuotes: "\"'",
	},
	"json": {
		Types:        set("true false null"),
		StringQuotes: "\"",
	},
	"yaml": {
		Types:        set("true false null yes no"),
		LineComment:  []string{"#"},
		StringQuotes: "\"'",
	},
}

// aliases keep one lexer per family. A separate "javascript" table would be
// the same table with a different name, and the second copy is where they
// drift.
var aliases = map[string]string{
	"golang": "go", "rs": "rust",
	"js": "ts", "javascript": "ts", "typescript": "ts", "jsx": "ts", "tsx": "ts",
	"py": "python", "postgres": "sql", "postgresql": "sql", "psql": "sql",
	"sh": "bash", "shell": "bash", "zsh": "bash", "console": "bash",
	"yml": "yaml", "proto": "go", "protobuf": "go",
}

// Languages is every name Highlight recognises, sorted — so a UI can say
// what it supports instead of guessing.
func Languages() []string {
	out := make([]string, 0, len(langs)+len(aliases))
	for k := range langs {
		out = append(out, k)
	}
	for k := range aliases {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func set(words string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(words) {
		m[w] = true
	}
	return m
}

// Highlight tokenises src as lang.
//
// An UNKNOWN language is not an error and not plain text: it falls back to a
// lexer that still finds strings, numbers and // # -- comments, because those
// three are right in almost every language and a code block with none of them
// coloured looks broken rather than unsupported.
func Highlight(lang, src string) []Token {
	name := strings.ToLower(strings.TrimSpace(lang))
	if a, ok := aliases[name]; ok {
		name = a
	}
	l, known := langs[name]
	if !known {
		l = Lang{
			LineComment:  []string{"//", "#", "--"},
			BlockOpen:    "/*",
			BlockClose:   "*/",
			StringQuotes: "\"'`",
		}
	}

	var out []Token
	emit := func(k Kind, text string) {
		if text == "" {
			return
		}
		// Merge adjacent same-kind runs. Without it a line of punctuation is
		// forty single-character spans, which is forty DOM nodes per line.
		if n := len(out); n > 0 && out[n-1].Kind == k {
			out[n-1].Text += text
			return
		}
		out = append(out, Token{Kind: k, Text: text})
	}

	r := []rune(src)
	i := 0
	for i < len(r) {
		c := r[i]

		// Comments, line and block.
		if lc, n := matchLineComment(r, i, l.LineComment); n > 0 {
			_ = lc
			j := i
			for j < len(r) && r[j] != '\n' {
				j++
			}
			emit(Comment, string(r[i:j]))
			i = j
			continue
		}
		if l.BlockOpen != "" && hasPrefixAt(r, i, l.BlockOpen) {
			j := i + len([]rune(l.BlockOpen))
			for j < len(r) && !hasPrefixAt(r, j, l.BlockClose) {
				j++
			}
			if j < len(r) {
				j += len([]rune(l.BlockClose))
			}
			// An UNTERMINATED block comment runs to end of input rather than
			// resetting: a lexer that cannot advance past a malformed token
			// hangs, and this is the shape that would.
			emit(Comment, string(r[i:j]))
			i = j
			continue
		}

		// Strings. Raw quotes ignore escapes; the others honour a backslash.
		if strings.ContainsRune(l.RawQuotes, c) && l.RawQuotes != "" {
			j := i + 1
			for j < len(r) && r[j] != c {
				j++
			}
			if j < len(r) {
				j++
			}
			emit(String, string(r[i:j]))
			i = j
			continue
		}
		if strings.ContainsRune(l.StringQuotes, c) && l.StringQuotes != "" {
			j := i + 1
			for j < len(r) {
				if r[j] == '\\' && j+1 < len(r) {
					j += 2
					continue
				}
				if r[j] == c || r[j] == '\n' {
					break
				}
				j++
			}
			// Stop at a newline as well as at the closing quote: an
			// unterminated string must not swallow the rest of the block,
			// which is the single most visible highlighter failure there is.
			if j < len(r) && r[j] == c {
				j++
			}
			emit(String, string(r[i:j]))
			i = j
			continue
		}

		// Numbers. Leading digit only — an identifier may contain digits and
		// must not become one.
		if unicode.IsDigit(c) {
			j := i
			for j < len(r) && (unicode.IsDigit(r[j]) || r[j] == '.' || r[j] == '_' ||
				r[j] == 'x' || r[j] == 'X' || r[j] == 'b' ||
				(r[j] >= 'a' && r[j] <= 'f') || (r[j] >= 'A' && r[j] <= 'F')) {
				j++
			}
			emit(Number, string(r[i:j]))
			i = j
			continue
		}

		// Identifiers, and the one heuristic in the whole file: an identifier
		// immediately followed by "(" is a call. It is wrong for `if (x)` in
		// C-like languages, which is why `if` is checked as a keyword first.
		if unicode.IsLetter(c) || c == '_' {
			j := i
			for j < len(r) && (unicode.IsLetter(r[j]) || unicode.IsDigit(r[j]) || r[j] == '_') {
				j++
			}
			word := string(r[i:j])
			switch {
			case l.Keywords[word]:
				emit(Keyword, word)
			case l.Types[word]:
				emit(Type, word)
			case peekNonSpace(r, j) == '(':
				emit(Func, word)
			default:
				emit(Plain, word)
			}
			i = j
			continue
		}

		if strings.ContainsRune("{}()[];,.:=+-*/<>!&|%^~?", c) {
			emit(Punct, string(c))
			i++
			continue
		}

		emit(Plain, string(c))
		i++
	}
	return out
}

func matchLineComment(r []rune, i int, prefixes []string) (string, int) {
	for _, p := range prefixes {
		if p != "" && hasPrefixAt(r, i, p) {
			return p, len([]rune(p))
		}
	}
	return "", 0
}

func hasPrefixAt(r []rune, i int, p string) bool {
	pr := []rune(p)
	if i+len(pr) > len(r) {
		return false
	}
	for k, c := range pr {
		if r[i+k] != c {
			return false
		}
	}
	return true
}

func peekNonSpace(r []rune, i int) rune {
	for i < len(r) && (r[i] == ' ' || r[i] == '\t') {
		i++
	}
	if i >= len(r) {
		return 0
	}
	return r[i]
}
