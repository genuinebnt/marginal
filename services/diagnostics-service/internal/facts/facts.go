// Package facts makes facts.html real: "define a value once, reference
// it anywhere, and every reference is flagged stale the moment the
// definition changes" (ROADMAP.md § Fact dependency graph). Pure
// functions over already-read block text, no I/O — same discipline as
// internal/analyzers and graphalgo.
//
// Concrete syntax (this repo's own concretization of RFC-003 §4's
// "define p99-latency = 180ms... reference it as a transclusion";
// neither RFC-003 nor ROADMAP.md pins down a literal syntax):
//
//	{{define name = value}}   — a whole block's entire text: a definition
//	{{name}}                  — anywhere else: a reference/transclusion
//
// A definition's own value may itself reference other definitions
// ("a = {{b}}, b = {{a}}" — ROADMAP's own cycle example), which is what
// makes this a dependency DAG rather than a flat lookup table.
package facts

import (
	"regexp"
	"strings"

	"marginal/documentcore"
	"marginal/graphalgo"
)

// definePattern matches a block whose entire text is a definition —
// deliberately anchored (^...$) so a definition can't hide inside a
// longer sentence; a name is a plain identifier, matching the same
// character class [[link]] titles don't need to restrict but a
// transclusion name benefits from (no risk of a name containing "}}").
var definePattern = regexp.MustCompile(`^\{\{define\s+([A-Za-z][A-Za-z0-9_-]*)\s*=\s*(.*)\}\}$`)

// referencePattern matches a plain {{name}} mention, in prose or inside
// a definition's own value.
var referencePattern = regexp.MustCompile(`\{\{([A-Za-z][A-Za-z0-9_-]*)\}\}`)

// Definition is one {{define name = value}} block.
type Definition struct {
	Name    string
	Value   string
	PageID  string
	BlockID documentcore.BlockID
}

// Reference is one {{name}} mention that is not itself a definition —
// either ordinary prose transcluding a value, or found inside another
// definition's own value (dependency edges are built from these too).
type Reference struct {
	Name    string
	PageID  string
	BlockID documentcore.BlockID
}

// PageBlocks is one page's blocks, as this package needs them to scan —
// the same shape analyzers.Page reduces to, kept separate so this
// package has no dependency on that one (each depends only on
// documentcore, matching graphalgo's own "no dependency on any one
// consumer" discipline).
type PageBlocks struct {
	PageID string
	Blocks []struct {
		ID   documentcore.BlockID
		Text string
	}
}

// Graph is the whole facts dependency DAG, built once from every page's
// blocks (Build). Definitions holds only definitions that survived
// duplicate and cycle rejection — a name with either problem is excluded
// from lookups (its own diagnostic still reports it; see Duplicates/Cycle).
type Graph struct {
	Definitions map[string]Definition
	// Duplicates maps a name to every definition claiming it, only for
	// names with more than one — "two definitions of one name is a
	// hash-lookup collision, not a satisfiability problem" (ROADMAP.md).
	Duplicates map[string][]Definition
	// Cycle is one example cycle of fact names ("a = {{b}}, b = {{a}}"),
	// or nil if the dependency DAG is acyclic — three-colour DFS,
	// graphalgo.DetectCycle reused unchanged ("the analyzer already has
	// it").
	Cycle []string
	// References is every {{name}} mention found anywhere that isn't
	// itself a definition — both plain prose transclusions and the
	// nested references inside other definitions' own values (needed so
	// StaleReferences can walk from one definition into another).
	References []Reference

	depGraph graphalgo.Graph // fact-name nodes; edge X->Y means "Y's value/reference depends on X" — dirty-mark propagation walks this forward
}

// Build scans every page's blocks for definitions and references and
// assembles the dependency DAG. A name is rejected from Definitions (but
// still reported via Duplicates) if more than one block defines it, and
// separately excluded (via Cycle) if it participates in a dependency
// cycle — both rejections leave the raw data inspectable, never silently
// dropped.
func Build(pages []PageBlocks) Graph {
	byName := make(map[string][]Definition)
	var refs []Reference
	var depEdges []graphalgo.Edge
	var factNodes []graphalgo.NodeID
	seenNode := make(map[string]bool)

	addNode := func(name string) {
		if !seenNode[name] {
			seenNode[name] = true
			factNodes = append(factNodes, graphalgo.NodeID(name))
		}
	}

	for _, page := range pages {
		for _, b := range page.Blocks {
			if m := definePattern.FindStringSubmatch(strings.TrimSpace(b.Text)); m != nil {
				name, value := m[1], m[2]
				def := Definition{Name: name, Value: value, PageID: page.PageID, BlockID: b.ID}
				byName[name] = append(byName[name], def)
				addNode(name)
				for _, dep := range referencePattern.FindAllStringSubmatch(value, -1) {
					depName := dep[1]
					addNode(depName)
					depEdges = append(depEdges, graphalgo.Edge{From: graphalgo.NodeID(depName), To: graphalgo.NodeID(name)})
					refs = append(refs, Reference{Name: depName, PageID: page.PageID, BlockID: b.ID})
				}
				continue
			}
			for _, m := range referencePattern.FindAllStringSubmatch(b.Text, -1) {
				name := m[1]
				addNode(name)
				refs = append(refs, Reference{Name: name, PageID: page.PageID, BlockID: b.ID})
			}
		}
	}

	depGraph := graphalgo.Graph{Nodes: factNodes, Edges: depEdges}
	cycleIDs := graphalgo.DetectCycle(depGraph)
	onCycle := make(map[string]bool, len(cycleIDs))
	var cycle []string
	for _, id := range cycleIDs {
		onCycle[string(id)] = true
		cycle = append(cycle, string(id))
	}

	definitions := make(map[string]Definition, len(byName))
	duplicates := make(map[string][]Definition)
	for name, defs := range byName {
		if len(defs) > 1 {
			duplicates[name] = defs
			continue
		}
		if onCycle[name] {
			continue
		}
		definitions[name] = defs[0]
	}

	return Graph{
		Definitions: definitions,
		Duplicates:  duplicates,
		Cycle:       cycle,
		References:  refs,
		depGraph:    depGraph,
	}
}

// StaleReferences returns every Reference whose own name is factName, or
// that transitively depends on factName through some chain of
// definitions' values — "mark the definition dirty, propagate forward
// through the dependency DAG in topological order, re-check only what is
// genuinely downstream" (ROADMAP.md), via graphalgo.ForwardReachable
// reused unchanged: a directed forward walk from factName over depGraph
// visits exactly the names genuinely downstream of it.
func (g Graph) StaleReferences(factName string) []Reference {
	downstream := graphalgo.ForwardReachable(g.depGraph, graphalgo.NodeID(factName))
	var out []Reference
	for _, ref := range g.References {
		if _, ok := downstream[graphalgo.NodeID(ref.Name)]; ok {
			out = append(out, ref)
		}
	}
	return out
}
