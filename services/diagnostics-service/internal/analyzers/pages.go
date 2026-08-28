package analyzers

import "marginal/graphalgo"

// DuplicateTitle is RFC-003 §2's DuplicateTitle: two pages share a
// title, which makes every [[link]] naming it ambiguous — the analyzer
// ROADMAP.md § Fact dependency graph calls "a hash-lookup collision, not
// a satisfiability problem": ctx.Resolve already groups pages by
// case-folded title, so a duplicate is just a Resolve on page's own
// title coming back Ambiguous.
func DuplicateTitle(page Page, ctx Context) []Diagnostic {
	res, matches := ctx.Resolve(page.Title)
	if res != Ambiguous {
		return nil
	}
	return []Diagnostic{{
		Analyzer: NameDuplicateTitle,
		Severity: Warning,
		Message:  formatDuplicateTitleMessage(page.Title, len(matches)),
	}}
}

func formatDuplicateTitleMessage(title string, count int) string {
	if count == 2 {
		return "Another page has this same title"
	}
	return "Several other pages share this title"
}

// OrphanPage is RFC-003 §2's OrphanPage: nothing links here and it is
// not a root — graphalgo.Components/Orphans, unchanged, the same
// connected-components reasoning graph-algorithms.html already argues
// for over a naive "backlinks == 0" check (graphalgo.Orphans' own doc
// comment has the full argument: a mutually-linked pair is just as
// orphaned as one unlinked page, even though each page in it has a
// nonzero backlink count individually).
func OrphanPage(page Page, ctx Context) []Diagnostic {
	comp := graphalgo.Components(ctx.Graph)
	var roots []graphalgo.NodeID
	for _, p := range ctx.Pages {
		if p.IsRoot {
			roots = append(roots, graphalgo.NodeID(p.ID))
		}
	}
	orphanComponents := graphalgo.Orphans(comp, roots)

	pageComponent, ok := comp[graphalgo.NodeID(page.ID)]
	if !ok {
		return nil // page isn't in the graph at all (no nodes) — nothing to say
	}
	for _, oc := range orphanComponents {
		if oc == pageComponent {
			return []Diagnostic{{
				Analyzer: NameOrphanPage,
				Severity: Info,
				Message:  "Nothing links to this page",
			}}
		}
	}
	return nil
}
