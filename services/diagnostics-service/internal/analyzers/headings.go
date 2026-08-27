package analyzers

import (
	"fmt"

	"marginal/documentcore"
)

// HeadingSkips is RFC-003 §2's HeadingSkip: H1 -> H3 with no H2 between
// them. Walks page's headings in document order (already position-
// ordered by PageService.ListBlocks) and flags whenever a heading's
// level jumps by more than one from the immediately preceding heading —
// H2 -> H2, H2 -> H1, and H1 -> H2 are all fine; H1 -> H3 is not, and
// neither is H2 -> H3 -> nothing-in-between... wait, H2 -> H3 is a jump
// of exactly one and is fine. Only a jump of two (H1 -> H3) is flagged,
// matching RFC-003's own example.
func HeadingSkips(page Page) []Diagnostic {
	var out []Diagnostic
	lastLevel := uint8(0)
	for _, b := range page.Blocks {
		if b.Kind.Tag != documentcore.Heading {
			continue
		}
		level := b.Kind.Level
		if lastLevel > 0 && level > lastLevel+1 {
			id := b.ID
			out = append(out, Diagnostic{
				Analyzer: NameHeadingSkip,
				Severity: Hint,
				Message:  fmt.Sprintf("Heading level %d follows level %d with none in between", level, lastLevel),
				BlockID:  &id,
			})
		}
		lastLevel = level
	}
	return out
}
