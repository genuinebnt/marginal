package trie

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrefixSearchFindsMatchingTitles(t *testing.T) {
	var tr Trie
	for _, title := range []string{"Architecture notes", "Architecture decisions", "Rollout plan"} {
		tr.Insert(title)
	}
	got := tr.PrefixSearch("Arch")
	sort.Strings(got)
	assert.Equal(t, []string{"Architecture decisions", "Architecture notes"}, got)
}

// TestPrefixSearchIsCaseInsensitive is the exact bug an inconsistent
// Insert/PrefixSearch casing produces: a title stored as typed
// ("Architecture", capital A) must still be found by a lowercase query
// ("arch") — [[ autocomplete has no reason to require a user match the
// original title's exact casing while typing.
func TestPrefixSearchIsCaseInsensitive(t *testing.T) {
	var tr Trie
	tr.Insert("Architecture notes")
	assert.Equal(t, []string{"Architecture notes"}, tr.PrefixSearch("arch"))
	assert.Equal(t, []string{"Architecture notes"}, tr.PrefixSearch("ARCH"))
	assert.Equal(t, []string{"Architecture notes"}, tr.PrefixSearch("Architecture notes"))
}

func TestPrefixSearchReturnsOriginalCasingNotTheQuerysOwn(t *testing.T) {
	var tr Trie
	tr.Insert("Architecture Notes")
	got := tr.PrefixSearch("architecture")
	require := assert.New(t)
	require.Len(got, 1)
	require.Equal("Architecture Notes", got[0])
}

func TestPrefixSearchOnEmptyTrieReturnsNil(t *testing.T) {
	var tr Trie
	assert.Nil(t, tr.PrefixSearch("anything"))
}

func TestPrefixSearchWithNoMatchesReturnsNil(t *testing.T) {
	var tr Trie
	tr.Insert("Rollout plan")
	assert.Nil(t, tr.PrefixSearch("zzz"))
}

// TestPrefixSearchHandlesDuplicateTitles pins docs.pages' own stated
// contract (DATA_MODEL.md: "title uniqueness is NOT enforced") — two
// pages sharing one title must both come back, not just one.
func TestPrefixSearchHandlesDuplicateTitles(t *testing.T) {
	var tr Trie
	tr.Insert("Untitled")
	tr.Insert("Untitled")
	got := tr.PrefixSearch("Untitled")
	assert.Equal(t, []string{"Untitled", "Untitled"}, got)
}

func TestEmptyPrefixReturnsEveryTitle(t *testing.T) {
	var tr Trie
	tr.Insert("A")
	tr.Insert("B")
	got := tr.PrefixSearch("")
	sort.Strings(got)
	assert.Equal(t, []string{"A", "B"}, got)
}
