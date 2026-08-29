package mdc

import "strings"

// lower turns tokens into the block tree.
//
// This is where the GRAMMAR is enforced (RFC-001, and the handbook's G4.3 /
// G4.5): a List gets ListItem children and nothing else, and a nested list is
// a List inside a ListItem — never a ListItem inside a ListItem. That second
// rule is the one an implementation gets wrong when it treats indentation as
// a number instead of as structure, and it renders identically at one level
// of depth, which is why it survives until someone nests twice.
//
// INVARIANT W1 — lowering is total. Every token maps to something. There is
// no token this pass can be handed that it has no place for, which is why it
// returns diagnostics rather than an error.
func lower(tokens []Token) (Tree, []Diagnostic) {
	l := &lowerer{next: 0}
	diags := []Diagnostic{}

	i := 0
	line := 0
	for i < len(tokens) {
		tok := tokens[i]
		line++

		switch tok.Kind {
		case TokBlank:
			i++

		case TokDivider:
			l.emit("", BlockKind{Tag: "divider"}, PlainContent(""))
			i++

		case TokATXHeading:
			l.emit("", BlockKind{Tag: "heading", Level: tok.Level}, ParseInline(tok.Text))
			i++

		case TokFenceOpen:
			// A fence runs to its close or to the end of input. Unterminated
			// is a DIAGNOSTIC, not a failure: the text is still code, which
			// is the reading that loses nothing.
			var body []string
			j := i + 1
			for j < len(tokens) && tokens[j].Kind == TokCodeText {
				body = append(body, tokens[j].Text)
				j++
			}
			closed := j < len(tokens) && tokens[j].Kind == TokFenceClose
			if !closed {
				diags = append(diags, Diagnostic{
					Line:    line,
					Message: "code fence is never closed — the rest of the input is treated as code",
				})
			}
			// Code has no marks. Not "marks are ignored inside code" — the
			// content is constructed without them, so the state where a code
			// block carries one is not representable here (RFC-001 B3.5).
			l.emit("", BlockKind{Tag: "code_block", Language: tokens[i].Lang},
				PlainContent(strings.Join(body, "\n")))
			i = j
			if closed {
				i++
			}

		case TokQuote:
			// A quote is a CONTAINER: its prose is a child paragraph, not its
			// own text. Consecutive quote lines join into one quote.
			var lines []string
			j := i
			for j < len(tokens) && tokens[j].Kind == TokQuote {
				lines = append(lines, tokens[j].Text)
				j++
			}
			id := l.emit("", BlockKind{Tag: "quote"}, PlainContent(""))
			l.emit(id, BlockKind{Tag: "paragraph"}, ParseInline(strings.Join(lines, " ")))
			i = j

		case TokBullet, TokOrdered, TokTodo:
			i = l.lowerList(tokens, i)

		default: // TokParagraph, TokCodeText outside a fence
			// Consecutive paragraph lines are ONE paragraph: a hard wrap in
			// the source is not a paragraph break, and treating it as one is
			// the single most common way an importer mangles prose.
			var lines []string
			j := i
			for j < len(tokens) && tokens[j].Kind == TokParagraph {
				lines = append(lines, tokens[j].Text)
				j++
			}
			l.emit("", BlockKind{Tag: "paragraph"}, ParseInline(strings.Join(lines, " ")))
			i = j
		}
	}

	return Tree{Blocks: l.blocks}, diags
}

type lowerer struct {
	blocks []Block
	next   int
}

func (l *lowerer) emit(parent string, kind BlockKind, content Content) string {
	id := idFor(l.next)
	l.next++
	l.blocks = append(l.blocks, Block{ID: id, Parent: parent, Kind: kind, Content: content})
	return id
}

func listKindOf(k TokenKind) string {
	switch k {
	case TokOrdered:
		return "numbered"
	case TokTodo:
		return "todo"
	default:
		return "bulleted"
	}
}

// lowerList consumes one list, including nested ones, and returns the index
// after it.
//
// The nesting rule, made structural: a deeper item does not become a child
// ITEM — it becomes a child LIST of the item above it, which then holds the
// item. That is G4.5, and doing it any other way makes indentation a number
// the renderer has to interpret rather than a shape it can walk.
func (l *lowerer) lowerList(tokens []Token, i int) int {
	return l.lowerListAt(tokens, i, tokens[i].Indent, "")
}

func (l *lowerer) lowerListAt(tokens []Token, i, indent int, parent string) int {
	kind := listKindOf(tokens[i].Kind)
	listID := l.emit(parent, BlockKind{Tag: "list", ListKind: kind}, PlainContent(""))

	for i < len(tokens) {
		tok := tokens[i]
		if !isListToken(tok.Kind) {
			break
		}
		if tok.Indent < indent {
			break // belongs to an enclosing list
		}
		if tok.Indent > indent {
			// Deeper: a nested LIST under the item just emitted. An orphan
			// deeper item with no item above it (a document starting
			// indented) attaches to this list rather than being dropped.
			owner := l.lastChildOf(listID)
			if owner == "" {
				owner = l.emit(listID, BlockKind{Tag: "list_item"}, PlainContent(""))
			}
			i = l.lowerListAt(tokens, i, tok.Indent, owner)
			continue
		}
		if listKindOf(tok.Kind) != kind {
			break // a different list kind starts a different list
		}
		l.emit(listID, BlockKind{Tag: "list_item", Checked: tok.Checked}, ParseInline(tok.Text))
		i++
	}
	return i
}

func (l *lowerer) lastChildOf(parent string) string {
	for k := len(l.blocks) - 1; k >= 0; k-- {
		if l.blocks[k].Parent == parent {
			return l.blocks[k].ID
		}
	}
	return ""
}

func isListToken(k TokenKind) bool {
	return k == TokBullet || k == TokOrdered || k == TokTodo
}
