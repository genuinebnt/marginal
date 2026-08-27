// Package analyzers makes RFC-003 §2's analyzer table real: nine pure
// functions over a page's block tree plus a resolution context (the
// "symbol table" RFC-003 §3 describes) — no I/O, no database, matching
// the same dependency-free-package discipline graphalgo already
// established for graph.html/graph-algorithms.html. The caller
// (diagnostics-service's own gRPC layer) builds the Context from
// document-service's PageService/GraphService and ships whichever
// diagnostics result the client asked for; every law here is a plain
// unit test.
//
// Computed fresh per request, not incrementally memoised — RFC-003 §4
// frames incrementality as a correctness requirement at IDE scale
// (rust-analyzer, salsa), but at this repo's demo scale (a handful of
// pages, small blocks) a full recompute is fast enough in practice that
// building a query-memoisation engine to match would be exactly the
// speculative infrastructure this repo's speed rules say to skip. Stated
// plainly, not silently: this is the one place v2.3.0 knowingly falls
// short of RFC-003's own stated bar, matching this codebase's existing
// convention of documenting a real gap rather than papering over it.
package analyzers

import "marginal/documentcore"

type Severity string

const (
	Hint    Severity = "hint"
	Warning Severity = "warning"
	Info    Severity = "info"
)

// Name is one of RFC-003 §2's nine analyzer names, verbatim.
type Name string

const (
	NameDanglingPageLink  Name = "DanglingPageLink"
	NameAmbiguousPageLink Name = "AmbiguousPageLink"
	NameHeadingSkip       Name = "HeadingSkip"
	NameEmptyCodeBlock    Name = "EmptyCodeBlock"
	NameDuplicateTitle    Name = "DuplicateTitle"
	NameOrphanPage        Name = "OrphanPage"
	NameBrokenImage       Name = "BrokenImage"
	NameSelfLink          Name = "SelfLink"
	NameLinkCycle         Name = "LinkCycle"
)

// Diagnostic is RFC-003 §2's own row. BlockID is nil for a page-level
// diagnostic (DuplicateTitle, OrphanPage, LinkCycle) — nothing about
// those points at one specific block.
type Diagnostic struct {
	Analyzer Name
	Severity Severity
	Message  string
	BlockID  *documentcore.BlockID
}

// Block is one page's block, as this package needs it. Kind/Content are
// already unmarshalled from PageService.ListBlocks' own JSON bytes —
// documentcore.BlockKind/Content are shared with document-service via
// the marginal/documentcore module, so there is no local re-modelling of
// the wire shape, only of which fields an analyzer reads.
type Block struct {
	ID      documentcore.BlockID
	Parent  *documentcore.BlockID
	Kind    documentcore.BlockKind
	Content documentcore.Content
}

// Page is the page under analysis: its own identity plus its blocks in
// document order (PageService.ListBlocks' own ORDER BY position).
type Page struct {
	ID     string
	Title  string
	Blocks []Block
}

// AnalyzeAll runs every analyzer over page and concatenates the results,
// in RFC-003 §2's own table order — determinism (RFC-003 §8's first law)
// falls out of always running analyzers in the same order over the same
// (already block-position-ordered) input, not from any explicit sort.
func AnalyzeAll(page Page, ctx Context) []Diagnostic {
	var out []Diagnostic
	out = append(out, DanglingAndAmbiguousPageLinks(page, ctx)...)
	out = append(out, SelfLinks(page)...)
	out = append(out, HeadingSkips(page)...)
	out = append(out, EmptyCodeBlocks(page)...)
	out = append(out, BrokenImages(page)...)
	out = append(out, DuplicateTitle(page, ctx)...)
	out = append(out, OrphanPage(page, ctx)...)
	out = append(out, LinkCycle(page, ctx)...)
	return out
}
