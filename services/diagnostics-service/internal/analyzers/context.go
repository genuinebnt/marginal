package analyzers

import (
	"strings"

	"marginal/graphalgo"
)

// PageInfo is what the symbol table needs to know about every page on
// the instance (RFC-003 §3) — enough for the cross-page analyzers
// (DanglingPageLink, AmbiguousPageLink, DuplicateTitle, OrphanPage,
// LinkCycle all read this, never a page's own block content).
type PageInfo struct {
	ID     string
	Title  string
	IsRoot bool
}

// Context is every analyzer's resolution context: every page's identity
// (the symbol table) plus the whole link graph, reused as-is from
// graphalgo rather than rebuilt here — LinkCycle's own three-colour DFS
// is graphalgo.DetectCycle, unchanged, the same "the analyzer already
// has it" reuse ROADMAP.md's own Fact dependency graph section names for
// the facts DAG below.
type Context struct {
	Pages []PageInfo
	Graph graphalgo.Graph
}

// Resolution is RFC-003 §3's own ReferenceResolver result.
type Resolution int

const (
	Missing Resolution = iota
	Unique
	Ambiguous
)

// Resolve looks up title (case-insensitively, matching blockproj's own
// `lower(title) = lower(...)` link-resolution convention) against every
// page's title.
func (c Context) Resolve(title string) (Resolution, []PageInfo) {
	var matches []PageInfo
	for _, p := range c.Pages {
		if strings.EqualFold(p.Title, title) {
			matches = append(matches, p)
		}
	}
	switch len(matches) {
	case 0:
		return Missing, nil
	case 1:
		return Unique, matches
	default:
		return Ambiguous, matches
	}
}
