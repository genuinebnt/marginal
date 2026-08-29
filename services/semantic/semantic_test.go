package semantic_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/semantic"
)

func corpusOf(texts map[string]string) (*semantic.Corpus, map[string][]string) {
	terms := map[string][]string{}
	docs := make([]semantic.Document, 0, len(texts))
	for id, text := range texts {
		t := semantic.Tokenize(text)
		terms[id] = t
		docs = append(docs, semantic.Document{ID: id, Terms: t})
	}
	return semantic.NewCorpus(docs), terms
}

func TestTokenizeDropsStopWordsAndPunctuation(t *testing.T) {
	got := semantic.Tokenize("The rope, and the B-tree: it splits!")
	assert.Equal(t, []string{"rope", "tree", "splits"}, got)
}

func TestEmbedIsNormalisedSoCosineIsADotProduct(t *testing.T) {
	c, terms := corpusOf(map[string]string{"a": "rope splits rebalance"})
	v := c.Embed(terms["a"])
	assert.InDelta(t, 1.0, semantic.Cosine(v, v), 1e-6, "a unit vector is exactly similar to itself")
}

func TestEmbedOfNoIndexableTermsIsZeroNotNaN(t *testing.T) {
	// A page of pure stop words, or an empty one. Normalising a zero vector
	// naively divides by zero and every later comparison becomes NaN, which
	// then sorts unpredictably and silently corrupts a whole result list.
	c, terms := corpusOf(map[string]string{"a": "the and of to", "b": "rope"})
	v := c.Embed(terms["a"])
	assert.Equal(t, 0.0, semantic.Cosine(v, c.Embed(terms["b"])))
	assert.False(t, semantic.Cosine(v, v) != semantic.Cosine(v, v), "must not be NaN")
}

func TestIdfMakesARareTermCountForMoreThanACommonOne(t *testing.T) {
	// Every document mentions "block"; only two mention "voronoi". Two pages
	// sharing "voronoi" must score above two sharing only "block" — that is
	// the entire job of the IDF weight, and the property most easily lost by
	// switching to raw term frequency.
	c, terms := corpusOf(map[string]string{
		"a": "block voronoi",
		"b": "block voronoi",
		"c": "block tree",
		"d": "block tree",
		"e": "block list",
		"f": "block queue",
	})
	rare := semantic.Cosine(c.Embed(terms["a"]), c.Embed(terms["b"]))
	common := semantic.Cosine(c.Embed(terms["c"]), c.Embed(terms["e"]))
	assert.Greater(t, rare, common)
}

// --- the index -------------------------------------------------------------

// spread builds n vectors along a line in term space, so nearness is
// predictable without depending on real prose.
func spread(n int) []semantic.Vector {
	out := make([]semantic.Vector, n)
	for i := range out {
		var v semantic.Vector
		v[i%semantic.Dim] = 1
		v[(i+1)%semantic.Dim] = 0.5
		// Normalise via a round trip through the public surface would be
		// nicer; this is a test fixture, and the index does not require unit
		// vectors to be correct, only to make Cosine mean what it says.
		norm := float32(1.1180339887)
		v[i%semantic.Dim] /= norm
		v[(i+1)%semantic.Dim] /= norm
		out[i] = v
	}
	return out
}

func TestSearchMatchesBruteForceOnASmallCorpus(t *testing.T) {
	// The one that matters. An approximate index nobody checks is an index
	// asking to be trusted; this asserts recall@5 is perfect on a corpus
	// small enough that anything less is a bug rather than a tradeoff.
	ix := semantic.NewIndex(8, 64, 0x5EED)
	vs := spread(120)
	for i, v := range vs {
		ix.Add(fmt.Sprintf("p%03d", i), v)
	}
	got, stats := ix.Search(vs[7], 5, 64, "p007", nil)
	require.Len(t, got, 5)
	assert.Equal(t, 1.0, stats.RecallAtK, "approximate answer must match the exact top-5")

	exact := ix.Exact(vs[7], 5, "p007", nil)
	for i := range exact {
		assert.Equal(t, exact[i].ID, got[i].ID, "and in the same order")
	}
}

func TestSearchNeverReturnsTheQueryPageItself(t *testing.T) {
	ix := semantic.NewIndex(8, 64, 1)
	vs := spread(40)
	for i, v := range vs {
		ix.Add(fmt.Sprintf("p%02d", i), v)
	}
	got, _ := ix.Search(vs[3], 10, 32, "p03", nil)
	for _, r := range got {
		assert.NotEqual(t, "p03", r.ID, "a page is not its own neighbour")
	}
}

func TestFilterRidingTheDescentStillReturnsKResults(t *testing.T) {
	// The hardest case here, and the reason the filter is inside searchLayer
	// rather than applied to its output. Only every seventh element passes,
	// so a post-filter over the top-5 would return zero or one. Filtering
	// during the descent must still come back with five.
	ix := semantic.NewIndex(8, 64, 7)
	vs := spread(140)
	for i, v := range vs {
		ix.Add(fmt.Sprintf("p%03d", i), v)
	}
	allow := func(id string) bool {
		var n int
		_, _ = fmt.Sscanf(id, "p%03d", &n)
		return n%7 == 0
	}
	got, stats := ix.Search(vs[0], 5, 64, "p000", allow)
	require.Len(t, got, 5, "a narrow filter must not silently shrink the result set")
	for _, r := range got {
		assert.True(t, allow(r.ID))
	}
	assert.Equal(t, 19, stats.Candidates, "139 others, every seventh, minus the excluded query")
	assert.Equal(t, 1.0, stats.RecallAtK)
}

func TestSearchIsCheaperThanAnExactScan(t *testing.T) {
	// The justification for the structure existing at all. If this ever
	// fails, the index is costing more than it saves and should be deleted
	// rather than tuned.
	ix := semantic.NewIndex(8, 64, 3)
	vs := spread(200)
	for i, v := range vs {
		ix.Add(fmt.Sprintf("p%03d", i), v)
	}
	_, stats := ix.Search(vs[100], 5, 32, "p100", nil)
	assert.Less(t, stats.Comparisons, stats.ExactComparisons)
}

func TestTheIndexIsDeterministicAcrossBuilds(t *testing.T) {
	// Level assignment is random. Seeded, two builds over identical input
	// must give identical answers — otherwise the recall number printed on
	// § 09 cannot be compared with the one printed a minute ago.
	build := func() []semantic.Result {
		ix := semantic.NewIndex(8, 64, 0xABCD)
		vs := spread(80)
		for i, v := range vs {
			ix.Add(fmt.Sprintf("p%02d", i), v)
		}
		got, _ := ix.Search(vs[11], 5, 32, "p11", nil)
		return got
	}
	assert.Equal(t, build(), build())
}

func TestAnEmptyIndexAnswersNothingRatherThanPanicking(t *testing.T) {
	ix := semantic.NewIndex(8, 64, 1)
	got, stats := ix.Search(semantic.Vector{}, 5, 32, "", nil)
	assert.Empty(t, got)
	assert.Equal(t, 0, stats.ExactComparisons)
}

func TestLayerSizesShrinkGoingUp(t *testing.T) {
	// The structural claim the whole thing rests on: layer 0 holds
	// everything, and each layer up holds strictly fewer. A tower that does
	// not narrow is a set of duplicate flat graphs.
	ix := semantic.NewIndex(8, 64, 0x1234)
	vs := spread(300)
	for i, v := range vs {
		ix.Add(fmt.Sprintf("p%03d", i), v)
	}
	sizes := ix.LayerSizes()
	require.Greater(t, len(sizes), 1, "300 elements must produce more than one layer")
	assert.Equal(t, 300, sizes[0])
	for i := 1; i < len(sizes); i++ {
		assert.Less(t, sizes[i], sizes[i-1], "layer %d must be smaller than %d", i, i-1)
	}
}
