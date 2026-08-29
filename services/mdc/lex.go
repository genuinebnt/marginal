// Package mdc is the paste-and-import pipeline: markdown-ish text in, a
// block tree and the ops that build it out.
//
// Four passes, each lossy on purpose, and the loss is the design:
//
//	bytes ──lex──▶ tokens ──parse──▶ AST ──lower──▶ block tree ──emit──▶ ops
//
// lex discards whitespace runs and keeps positions; parse discards positions
// and produces an AST rather than a parse tree; lower discards syntax, so
// nothing downstream can depend on whether the author wrote *x* or _x_; emit
// discards the tree shape.
//
// THE PROPERTY THIS EXISTS TO HOLD (§ 11's own claim, and the only one worth
// testing): replaying the emitted ops into an empty tree must equal the tree
// the pipeline produced directly. Compile checks it on every call and reports
// the answer rather than asserting it — which is what lets the screen say
// HOLDS because a comparison happened, not because we are confident.
//
// It is Go and not a TypeScript markdown library for the reason every other
// algorithm here is (CLAUDE.md): the view layer draws what Go computed. It is
// compiled to wasm because it runs on a paste and on every keystroke of the
// lab screen's editable buffer, and a network round trip is not a paste.
package mdc

import (
	"strings"
	"unicode/utf8"
)

// TokenKind is a block-level lexical class. The lexer is line-oriented: this
// grammar has no construct that needs a character-level scan to RECOGNISE,
// only to interpret, and a line scanner is the cheapest thing that can
// answer "what kind of block starts here".
type TokenKind string

const (
	TokATXHeading TokenKind = "ATX_HEADING"
	TokParagraph  TokenKind = "PARAGRAPH"
	TokQuote      TokenKind = "QUOTE"
	TokBullet     TokenKind = "BULLET"
	TokOrdered    TokenKind = "ORDERED"
	TokTodo       TokenKind = "TODO"
	TokFenceOpen  TokenKind = "FENCE_OPEN"
	TokCodeText   TokenKind = "CODE_TEXT"
	TokFenceClose TokenKind = "FENCE_CLOSE"
	TokDivider    TokenKind = "DIVIDER"
	TokBlank      TokenKind = "BLANK"
)

// Token is one line, classified, with its BYTE span into the source.
//
// Byte offsets, not rune indices — the source is UTF-8 and every downstream
// consumer (marks, anchors, the editor's own content) counts bytes. § 11
// prints chars and bytes side by side precisely because they differ, and a
// lexer that reported rune indices would hide the one divergence the screen
// exists to show.
type Token struct {
	Kind  TokenKind `json:"kind"`
	Start int       `json:"start"`
	End   int       `json:"end"`
	// Text is the line's CONTENT with its marker stripped — "## " gone from a
	// heading, "- " from a bullet. The marker is syntax and this is the pass
	// that throws syntax away.
	Text string `json:"text"`
	// Level is meaningful for ATX_HEADING (1..3), Indent for the list kinds
	// (how many levels deep), Lang for FENCE_OPEN, Checked for TODO.
	Level   int    `json:"level,omitempty"`
	Indent  int    `json:"indent,omitempty"`
	Lang    string `json:"lang,omitempty"`
	Checked bool   `json:"checked,omitempty"`
}

// MaxHeadingLevel is 3, not 6.
//
// The outline rail indents by level, and four levels of indent stop being
// legible in 272px — a level nobody can see is a level nobody should be able
// to write (RFC-001, and the handbook's G4.4). `#### x` lexes as a paragraph
// whose text still begins with the hashes, so nothing is silently eaten.
const MaxHeadingLevel = 3

// IndentWidth is how many leading spaces make one list level. Two, because a
// nested bullet written by hand is two spaces far more often than four, and
// tabs are expanded to it.
const IndentWidth = 2

// Lex classifies every line. It never fails and never returns early: a
// malformed construct becomes a paragraph, which is the only recovery that
// cannot lose text.
//
// INVARIANT L1 — the tokens tile the source exactly. Concatenating every
// token's [Start,End) covers [0,len(src)) with no gap and no overlap. That is
// what makes "bytes read" on § 11 a real number rather than an estimate, and
// it is checked by a test rather than assumed.
func Lex(src string) []Token {
	var out []Token
	offset := 0
	inFence := false
	fenceLang := ""

	for _, line := range splitLinesKeepingEnds(src) {
		start := offset
		end := offset + len(line)
		offset = end
		body := strings.TrimRight(line, "\r\n")
		trimmed := strings.TrimSpace(body)

		if inFence {
			if isFence(trimmed) {
				inFence = false
				fenceLang = ""
				out = append(out, Token{Kind: TokFenceClose, Start: start, End: end})
				continue
			}
			out = append(out, Token{Kind: TokCodeText, Start: start, End: end, Text: body})
			continue
		}

		if isFence(trimmed) {
			inFence = true
			fenceLang = strings.TrimSpace(strings.TrimLeft(trimmed, "`~"))
			out = append(out, Token{Kind: TokFenceOpen, Start: start, End: end, Lang: fenceLang})
			continue
		}

		if trimmed == "" {
			out = append(out, Token{Kind: TokBlank, Start: start, End: end})
			continue
		}

		if isDivider(trimmed) {
			out = append(out, Token{Kind: TokDivider, Start: start, End: end})
			continue
		}

		if level, text, ok := atxHeading(trimmed); ok {
			out = append(out, Token{Kind: TokATXHeading, Start: start, End: end, Level: level, Text: text})
			continue
		}

		if strings.HasPrefix(trimmed, "> ") || trimmed == ">" {
			out = append(out, Token{
				Kind: TokQuote, Start: start, End: end,
				Text: strings.TrimPrefix(strings.TrimPrefix(trimmed, ">"), " "),
			})
			continue
		}

		indent := leadingIndent(body)
		if checked, text, ok := todoItem(trimmed); ok {
			out = append(out, Token{Kind: TokTodo, Start: start, End: end, Indent: indent, Checked: checked, Text: text})
			continue
		}
		if text, ok := bulletItem(trimmed); ok {
			out = append(out, Token{Kind: TokBullet, Start: start, End: end, Indent: indent, Text: text})
			continue
		}
		if text, ok := orderedItem(trimmed); ok {
			out = append(out, Token{Kind: TokOrdered, Start: start, End: end, Indent: indent, Text: text})
			continue
		}

		out = append(out, Token{Kind: TokParagraph, Start: start, End: end, Text: trimmed})
	}

	// An unterminated fence is NOT an error and does not reset: the remaining
	// lines are already CODE_TEXT, which is the reading that loses nothing.
	// A lexer that could not advance past a malformed construct would hang,
	// and this is the shape that would.
	_ = fenceLang
	return out
}

// splitLinesKeepingEnds splits on "\n" but keeps the terminator on each line,
// so the spans tile the source (L1). strings.Split would drop them and every
// offset after the first line would be one byte short, cumulatively.
func splitLinesKeepingEnds(src string) []string {
	if src == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			out = append(out, src[start:i+1])
			start = i + 1
		}
	}
	if start < len(src) {
		out = append(out, src[start:])
	}
	return out
}

func isFence(s string) bool {
	return strings.HasPrefix(s, "```") || strings.HasPrefix(s, "~~~")
}

func isDivider(s string) bool {
	if len(s) < 3 {
		return false
	}
	c := s[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != c {
			return false
		}
	}
	return true
}

func atxHeading(s string) (level int, text string, ok bool) {
	n := 0
	for n < len(s) && s[n] == '#' {
		n++
	}
	if n == 0 || n > MaxHeadingLevel || n >= len(s) || s[n] != ' ' {
		return 0, "", false
	}
	return n, strings.TrimSpace(s[n+1:]), true
}

func bulletItem(s string) (string, bool) {
	if len(s) > 1 && (s[0] == '-' || s[0] == '*' || s[0] == '+') && s[1] == ' ' {
		return strings.TrimSpace(s[2:]), true
	}
	return "", false
}

func orderedItem(s string) (string, bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 || i+1 >= len(s) {
		return "", false
	}
	if (s[i] == '.' || s[i] == ')') && s[i+1] == ' ' {
		return strings.TrimSpace(s[i+2:]), true
	}
	return "", false
}

func todoItem(s string) (checked bool, text string, ok bool) {
	rest, isBullet := bulletItem(s)
	if !isBullet {
		return false, "", false
	}
	switch {
	case strings.HasPrefix(rest, "[ ] "):
		return false, strings.TrimSpace(rest[4:]), true
	case strings.HasPrefix(rest, "[x] "), strings.HasPrefix(rest, "[X] "):
		return true, strings.TrimSpace(rest[4:]), true
	}
	return false, "", false
}

func leadingIndent(line string) int {
	spaces := 0
	for _, r := range line {
		switch r {
		case ' ':
			spaces++
		case '\t':
			spaces += IndentWidth
		default:
			return spaces / IndentWidth
		}
	}
	return spaces / IndentWidth
}

// CountChars is the rune count. § 11 prints it beside the byte count because
// the two differ, and the difference is the whole reason offsets in this
// system are bytes.
func CountChars(s string) int { return utf8.RuneCountInString(s) }
