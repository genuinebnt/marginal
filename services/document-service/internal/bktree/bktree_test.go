package bktree

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestLevenshteinKnownDistances(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"kitten", "sitting", 3}, // the textbook example
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"flaw", "lawn", 2},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, Levenshtein(c.a, c.b), "Levenshtein(%q, %q)", c.a, c.b)
		assert.Equal(t, c.want, Levenshtein(c.b, c.a), "Levenshtein is symmetric")
	}
}

func TestQueryFindsExactMatchAtDistanceZero(t *testing.T) {
	var tree Tree
	for _, w := range []string{"Performance budget", "Pricing tiers", "Rollout plan"} {
		tree.Insert(w)
	}
	matches := tree.Query("Performance budget", 0)
	require.Len(t, matches, 1)
	assert.Equal(t, "Performance budget", matches[0].Word)
	assert.Equal(t, 0, matches[0].Distance)
}

func TestQueryFindsATypoWithinDistance(t *testing.T) {
	var tree Tree
	for _, w := range []string{"Performance budget", "Pricing tiers", "Rollout plan", "Architecture notes"} {
		tree.Insert(w)
	}
	// "Performnace" — a transposition, distance 2 from "Performance".
	matches := tree.Query("Performnace budget", 2)
	var words []string
	for _, m := range matches {
		words = append(words, m.Word)
	}
	assert.Contains(t, words, "Performance budget")
}

func TestQueryOnEmptyTreeReturnsNil(t *testing.T) {
	var tree Tree
	assert.Nil(t, tree.Query("anything", 5))
}

// TestQueryMatchesBruteForce is the actual correctness law a BK-tree's
// pruning must satisfy: querying the tree must return exactly the same
// set of words a linear scan (computing Levenshtein against every
// inserted word) would — the triangle-inequality pruning is a
// performance optimization, and it is worthless if it ever skips a
// subtree that actually contained a match. Checked across 100 random
// vocabularies and queries rather than trusted from the argument alone.
func TestQueryMatchesBruteForce(t *testing.T) {
	wordGen := rapid.StringMatching(`[a-c]{1,5}`)
	rapid.Check(t, func(rt *rapid.T) {
		words := rapid.SliceOfN(wordGen, 0, 12).Draw(rt, "words")
		query := wordGen.Draw(rt, "query")
		maxDistance := rapid.IntRange(0, 4).Draw(rt, "maxDistance")

		var tree Tree
		seen := map[string]bool{}
		var unique []string
		for _, w := range words {
			if !seen[w] {
				seen[w] = true
				unique = append(unique, w)
				tree.Insert(w)
			}
		}

		var want []string
		for _, w := range unique {
			if Levenshtein(w, query) <= maxDistance {
				want = append(want, w)
			}
		}
		sort.Strings(want)

		var got []string
		for _, m := range tree.Query(query, maxDistance) {
			got = append(got, m.Word)
			if Levenshtein(m.Word, query) != m.Distance {
				rt.Fatalf("Match.Distance %d does not match the real Levenshtein distance for %q vs %q", m.Distance, m.Word, query)
			}
		}
		sort.Strings(got)

		if len(want) != len(got) {
			rt.Fatalf("brute force found %v, tree found %v (query=%q, maxDistance=%d, words=%v)", want, got, query, maxDistance, unique)
		}
		for i := range want {
			if want[i] != got[i] {
				rt.Fatalf("brute force found %v, tree found %v (query=%q, maxDistance=%d, words=%v)", want, got, query, maxDistance, unique)
			}
		}
	})
}
