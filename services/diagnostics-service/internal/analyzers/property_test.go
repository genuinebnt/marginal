package analyzers

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"

	"marginal/documentcore"
	"marginal/graphalgo"
)

// TestAnalyzeAllNeverPanics is this package's own never-panic law
// (RFC-003 §8) — a page's block content and the resolution context both
// ultimately come from user-typed prose (page titles, [[link]] text,
// heading levels chosen by input rules), so an adversarial-shape check
// belongs here the same way it does for graphalgo (.agents/agents.md's
// untrusted-input discipline).
func TestAnalyzeAllNeverPanics(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 8).Draw(t, "n")
		var blocks []Block
		for i := 0; i < n; i++ {
			kind := rapid.SampledFrom([]documentcore.BlockKind{
				documentcore.NewParagraph(),
				documentcore.NewCodeBlock(""),
				mustHeading(rapid.IntRange(1, 3).Draw(t, "level")),
				documentcore.NewImage(documentcore.FileID{}),
			}).Draw(t, "kind")
			text := rapid.SampledFrom([]string{
				"", "[[A]]", "[[A]] [[B]]", "[[]]", "[[Self]]", "plain text",
			}).Draw(t, "text")
			blocks = append(blocks, Block{ID: newBlockID(), Kind: kind, Content: documentcore.Content{Text: text}})
		}

		pageCount := rapid.IntRange(1, 5).Draw(t, "pageCount")
		var pages []PageInfo
		var nodes []graphalgo.NodeID
		for i := 0; i < pageCount; i++ {
			id := fmt.Sprintf("p%d", i)
			pages = append(pages, PageInfo{ID: id, Title: rapid.SampledFrom([]string{"Self", "A", "B", "Self"}).Draw(t, "title"), IsRoot: i == 0})
			nodes = append(nodes, graphalgo.NodeID(id))
		}
		var edges []graphalgo.Edge
		numEdges := rapid.IntRange(0, pageCount*pageCount).Draw(t, "numEdges")
		for i := 0; i < numEdges; i++ {
			from := nodes[rapid.IntRange(0, pageCount-1).Draw(t, "from")]
			to := nodes[rapid.IntRange(0, pageCount-1).Draw(t, "to")]
			edges = append(edges, graphalgo.Edge{From: from, To: to})
		}

		page := Page{ID: "p0", Title: "Self", Blocks: blocks}
		ctx := Context{Pages: pages, Graph: graphalgo.Graph{Nodes: nodes, Edges: edges}}
		_ = AnalyzeAll(page, ctx)
	})
}

func mustHeading(level int) documentcore.BlockKind {
	kind, err := documentcore.NewHeading(uint8(level))
	if err != nil {
		panic(err)
	}
	return kind
}
