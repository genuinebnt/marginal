package mdc

import "strings"

// MarkTag mirrors documentcore's own set. Deliberately not imported from it:
// mdc is a pure module with no dependencies (the same rule graphalgo and
// semantic follow), and the wire shape is what the two have to agree on, not
// the type.
type MarkTag string

const (
	Bold     MarkTag = "bold"
	Italic   MarkTag = "italic"
	Strike   MarkTag = "strike"
	Code     MarkTag = "code"
	Link     MarkTag = "link"
	PageLink MarkTag = "page_link"
)

type MarkKind struct {
	Tag MarkTag `json:"tag"`
	URL string  `json:"url,omitempty"`
	// Page is the page link's TITLE as written, not an id: resolution
	// belongs to whoever has the page list, and a compiler that guessed an
	// id would be inventing a reference. blockproj resolves the same way.
	Page string `json:"page,omitempty"`
}

// Mark is a span of Content.Text, in BYTE offsets.
type Mark struct {
	Kind  MarkKind `json:"kind"`
	Start int      `json:"start"`
	End   int      `json:"end"`
}

// Content is a block's text plus its marks. Marks is never nil, so the JSON
// is always `"marks":[]` — the same rule documentcore.PlainContent follows,
// and for the same reason: a client should not need a null guard to iterate.
type Content struct {
	Text  string `json:"text"`
	Marks []Mark `json:"marks"`
}

// inlineDelims are the paired markers, longest first — `**` must be tried
// before `*`, or every bold opens as an italic and the close never matches.
var inlineDelims = []struct {
	open string
	tag  MarkTag
}{
	{"**", Bold},
	{"~~", Strike},
	{"`", Code},
	{"*", Italic},
	{"_", Italic},
}

// ParseInline turns one line of markdown-ish text into plain text plus marks
// over BYTE offsets into that text.
//
// INVARIANT I1 — the returned Text is the input with only the MARKERS
// removed. Nothing else is added, reordered or dropped, so a mark's offsets
// always land inside it.
//
// INVARIANT I2 — every offset is on a UTF-8 boundary. Markers are ASCII and
// are only ever removed whole, so this falls out rather than being enforced;
// a test pins it anyway, because it is the invariant whose violation is a
// rendering crash rather than a wrong colour.
//
// INVARIANT I3 — an UNMATCHED marker stays literal. `a * b` is a sentence,
// not an unterminated italic, and a parser that swallowed the rest of the
// line looking for a close would eat the paragraph.
//
// Nesting is deliberately NOT supported: `**bold *and italic* **` produces
// one bold mark and two literal asterisks. Supporting it needs a real
// recursive inline grammar, and RFC-001's marks are a flat, overlapping set
// — a flat parser is the honest match for a flat model.
func ParseInline(src string) Content {
	var b strings.Builder
	marks := []Mark{}
	i := 0

	for i < len(src) {
		// [[Page Title]] first: its brackets would otherwise be read as a
		// link's, and "[[" is a longer, more specific opener.
		if strings.HasPrefix(src[i:], "[[") {
			if end := strings.Index(src[i+2:], "]]"); end >= 0 {
				title := src[i+2 : i+2+end]
				start := b.Len()
				b.WriteString(title)
				marks = append(marks, Mark{
					Kind: MarkKind{Tag: PageLink, Page: strings.TrimSpace(title)},
					Start: start, End: b.Len(),
				})
				i += 2 + end + 2
				continue
			}
		}

		// [text](url)
		if src[i] == '[' {
			if text, url, n, ok := linkAt(src[i:]); ok {
				start := b.Len()
				b.WriteString(text)
				marks = append(marks, Mark{
					Kind:  MarkKind{Tag: Link, URL: url},
					Start: start, End: b.Len(),
				})
				i += n
				continue
			}
		}

		matched := false
		for _, d := range inlineDelims {
			if !strings.HasPrefix(src[i:], d.open) {
				continue
			}
			rest := src[i+len(d.open):]
			end := strings.Index(rest, d.open)
			// An empty body (`**` immediately closed) is two literal markers,
			// not an empty mark: a zero-width mark is unrenderable and
			// unselectable, so it must not be representable.
			if end <= 0 {
				continue
			}
			inner := rest[:end]
			start := b.Len()
			b.WriteString(inner)
			marks = append(marks, Mark{Kind: MarkKind{Tag: d.tag}, Start: start, End: b.Len()})
			i += len(d.open) + end + len(d.open)
			matched = true
			break
		}
		if matched {
			continue
		}

		// I3: nothing matched, so this byte is literal. Advancing by ONE
		// byte is safe here and only here — the loop never inspects a
		// partial rune, it copies one.
		b.WriteByte(src[i])
		i++
	}

	return Content{Text: b.String(), Marks: marks}
}

// linkAt parses `[text](url)` at the start of s, returning how many bytes it
// consumed. Anything malformed is not a link, and the caller falls through to
// treating `[` as literal — which is I3 again.
func linkAt(s string) (text, url string, n int, ok bool) {
	closeText := strings.Index(s, "]")
	if closeText < 0 || closeText+1 >= len(s) || s[closeText+1] != '(' {
		return "", "", 0, false
	}
	closeURL := strings.Index(s[closeText+2:], ")")
	if closeURL < 0 {
		return "", "", 0, false
	}
	return s[1:closeText], s[closeText+2 : closeText+2+closeURL], closeText + 2 + closeURL + 1, true
}

// PlainContent is text with no marks.
func PlainContent(text string) Content { return Content{Text: text, Marks: []Mark{}} }
