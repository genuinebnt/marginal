package analyzers

import "marginal/documentcore"

// EmptyCodeBlocks is RFC-003 §2's EmptyCodeBlock: a code block with no
// language set — the syntax-highlighting equivalent of an untitled tab,
// harmless but worth a nudge.
func EmptyCodeBlocks(page Page) []Diagnostic {
	var out []Diagnostic
	for _, b := range page.Blocks {
		if b.Kind.Tag != documentcore.CodeBlock {
			continue
		}
		if b.Kind.Language != "" {
			continue
		}
		id := b.ID
		out = append(out, Diagnostic{
			Analyzer: NameEmptyCodeBlock,
			Severity: Hint,
			Message:  "Code block has no language set",
			BlockID:  &id,
		})
	}
	return out
}

// BrokenImages is RFC-003 §2's BrokenImage: file_id resolves to nothing.
// This repo has no upload/asset pipeline yet (documentcore.FileID's own
// doc comment), so there is no real files table to check a file_id
// against — the honest, stated simplification here is narrower than
// RFC-003's own claim: this flags only the one case actually
// verifiable without one, a file_id left at its zero value (the
// placeholder web/'s own kindFromKey("image") fix already had to work
// around — see docs/porting/PROGRESS.md), not a genuinely-uploaded-then-
// deleted file. A real check needs a real files service; this is not it.
func BrokenImages(page Page) []Diagnostic {
	var out []Diagnostic
	var zero documentcore.FileID
	for _, b := range page.Blocks {
		if b.Kind.Tag != documentcore.Image {
			continue
		}
		if b.Kind.FileID != zero {
			continue
		}
		id := b.ID
		out = append(out, Diagnostic{
			Analyzer: NameBrokenImage,
			Severity: Warning,
			Message:  "Image has no file",
			BlockID:  &id,
		})
	}
	return out
}
