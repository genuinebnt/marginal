package analyzers

import (
	"fmt"
	"regexp"
	"strings"

	"marginal/graphalgo"
)

// pageLinkPattern matches [[Page Title]] — the exact regex
// internal/blockproj (document-service) already scans block text with,
// ported here rather than shared across a module boundary: it's four
// characters of regex, and duplicating it here is cheaper than the
// coupling a shared "linkscan" module would add for one constant.
var pageLinkPattern = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

// pageLinkMentions returns every [[title]] mention in text, in the order
// they appear.
func pageLinkMentions(text string) []string {
	matches := pageLinkPattern.FindAllStringSubmatch(text, -1)
	out := make([]string, len(matches))
	for i, m := range matches {
		out[i] = m[1]
	}
	return out
}

// DanglingAndAmbiguousPageLinks is one pass over page's own blocks
// (RFC-003 §2's DanglingPageLink and AmbiguousPageLink) — every [[link]]
// mention resolved against ctx's symbol table exactly once, since both
// analyzers ask the same question (what does this name resolve to?) and
// differ only in which Resolution they flag.
func DanglingAndAmbiguousPageLinks(page Page, ctx Context) []Diagnostic {
	var out []Diagnostic
	for _, b := range page.Blocks {
		for _, name := range pageLinkMentions(b.Content.Text) {
			id := b.ID
			switch res, matches := ctx.Resolve(name); res {
			case Missing:
				out = append(out, Diagnostic{
					Analyzer: NameDanglingPageLink,
					Severity: Hint,
					Message:  fmt.Sprintf("[[%s]] does not match any page", name),
					BlockID:  &id,
				})
			case Ambiguous:
				out = append(out, Diagnostic{
					Analyzer: NameAmbiguousPageLink,
					Severity: Warning,
					Message:  fmt.Sprintf("[[%s]] matches %d pages", name, len(matches)),
					BlockID:  &id,
				})
			case Unique:
				// resolves cleanly — nothing to report
			}
		}
	}
	return out
}

// SelfLinks is RFC-003 §2's SelfLink: a page linking to its own title.
// info severity — linking to yourself isn't wrong, just unusual enough
// to surface (matching RFC-003's own severity table, and the project's
// "nothing is ever broken" colour rule: info renders as a gutter marker
// only, not an underline).
func SelfLinks(page Page) []Diagnostic {
	var out []Diagnostic
	for _, b := range page.Blocks {
		for _, name := range pageLinkMentions(b.Content.Text) {
			if !strings.EqualFold(name, page.Title) {
				continue
			}
			id := b.ID
			out = append(out, Diagnostic{
				Analyzer: NameSelfLink,
				Severity: Info,
				Message:  fmt.Sprintf("This page links to itself: [[%s]]", name),
				BlockID:  &id,
			})
		}
	}
	return out
}

// LinkCycle is RFC-003 §2's LinkCycle: A -> B -> A, detected by
// three-colour DFS over the whole link graph — graphalgo.DetectCycle,
// unchanged, per ROADMAP.md § Fact dependency graph's own "three-colour
// DFS again — the analyzer already has it." Reported at the page level
// (BlockID nil) since a cycle is a property of the graph, not one block
// — a client wanting to point at the specific [[link]] responsible can
// cross-reference the cycle's own page-id sequence against this page's
// DanglingAndAmbiguousPageLinks scan.
func LinkCycle(page Page, ctx Context) []Diagnostic {
	cycle := graphalgo.DetectCycle(ctx.Graph)
	if cycle == nil {
		return nil
	}
	onCycle := false
	for _, id := range cycle {
		if string(id) == page.ID {
			onCycle = true
			break
		}
	}
	if !onCycle {
		return nil
	}
	return []Diagnostic{{
		Analyzer: NameLinkCycle,
		Severity: Info,
		Message:  "This page is part of a link cycle",
	}}
}
