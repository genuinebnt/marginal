package facts

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/documentcore"
)

func blockID() documentcore.BlockID { return documentcore.BlockID(uuid.Must(uuid.NewV7())) }

func page(id string, texts ...string) PageBlocks {
	p := PageBlocks{PageID: id}
	for _, t := range texts {
		p.Blocks = append(p.Blocks, struct {
			ID   documentcore.BlockID
			Text string
		}{ID: blockID(), Text: t})
	}
	return p
}

func TestBuildParsesADefinitionAndAReference(t *testing.T) {
	pages := []PageBlocks{
		page("perf", "{{define ack-budget = 40ms}}"),
		page("rollout", "Ship once {{ack-budget}} holds under load."),
	}
	g := Build(pages)

	require.Contains(t, g.Definitions, "ack-budget")
	assert.Equal(t, "40ms", g.Definitions["ack-budget"].Value)
	assert.Equal(t, "perf", g.Definitions["ack-budget"].PageID)

	require.Len(t, g.References, 1)
	assert.Equal(t, "ack-budget", g.References[0].Name)
	assert.Equal(t, "rollout", g.References[0].PageID)
}

func TestBuildRejectsDuplicateDefinitions(t *testing.T) {
	pages := []PageBlocks{
		page("a", "{{define plan-seats = 10 seats}}"),
		page("b", "{{define plan-seats = unlimited}}"),
	}
	g := Build(pages)

	assert.NotContains(t, g.Definitions, "plan-seats", "a duplicated name must not resolve to either definition")
	require.Contains(t, g.Duplicates, "plan-seats")
	assert.Len(t, g.Duplicates["plan-seats"], 2)
}

// TestBuildRejectsACycle pins ROADMAP.md's own example verbatim:
// "a = {{b}}, b = {{a}} is refused."
func TestBuildRejectsACycle(t *testing.T) {
	pages := []PageBlocks{
		page("p", "{{define a = about {{b}}}}", "{{define b = a fraction of {{a}}}}"),
	}
	g := Build(pages)

	assert.NotContains(t, g.Definitions, "a")
	assert.NotContains(t, g.Definitions, "b")
	require.NotNil(t, g.Cycle)
	assert.Contains(t, g.Cycle, "a")
	assert.Contains(t, g.Cycle, "b")
}

func TestBuildAcceptsANonCyclicChainOfDefinitions(t *testing.T) {
	// b depends on a, c depends on b — a real dependency chain, not a
	// cycle, must resolve cleanly end to end.
	pages := []PageBlocks{
		page("p", "{{define a = 40ms}}", "{{define b = about {{a}}}}", "{{define c = twice {{b}}}}"),
	}
	g := Build(pages)

	require.Contains(t, g.Definitions, "a")
	require.Contains(t, g.Definitions, "b")
	require.Contains(t, g.Definitions, "c")
	assert.Empty(t, g.Cycle)
}

// TestStaleReferencesWalksTheWholeDownstreamChain is ROADMAP.md's own
// "propagate forward through the dependency DAG in topological order,
// re-check only what is genuinely downstream" — b and c both depend on
// a (directly or transitively), and a plain prose reference to c must
// also come back as stale, since c's own value depends on a.
func TestStaleReferencesWalksTheWholeDownstreamChain(t *testing.T) {
	pages := []PageBlocks{
		page("defs", "{{define a = 40ms}}", "{{define b = about {{a}}}}", "{{define c = twice {{b}}}}"),
		page("doc", "See {{c}} for the final number."),
	}
	g := Build(pages)

	stale := g.StaleReferences("a")
	names := make([]string, len(stale))
	for i, r := range stale {
		names[i] = r.Name
	}
	assert.Contains(t, names, "b", "b's own value cites a directly")
	assert.Contains(t, names, "c", "c depends on b which depends on a — transitively stale")
}

func TestStaleReferencesIgnoresUnrelatedDefinitions(t *testing.T) {
	pages := []PageBlocks{
		page("defs", "{{define a = 1}}", "{{define unrelated = 2}}"),
		page("doc", "{{unrelated}}"),
	}
	g := Build(pages)

	stale := g.StaleReferences("a")
	for _, r := range stale {
		assert.NotEqual(t, "unrelated", r.Name)
	}
}

func TestPlainProseReferenceWithNoDefinitionIsStillRecorded(t *testing.T) {
	// A reference to a name nobody has defined yet — real content should
	// still surface it (as a genuinely undefined transclusion, the same
	// spirit as DanglingPageLink), not be silently dropped.
	pages := []PageBlocks{page("doc", "See {{not-yet-defined}}.")}
	g := Build(pages)

	require.Len(t, g.References, 1)
	assert.Equal(t, "not-yet-defined", g.References[0].Name)
	assert.NotContains(t, g.Definitions, "not-yet-defined")
}
