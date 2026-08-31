package discover

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"marginal/graphalgo"
)

// A tiny corpus with two clearly separate subjects, so a query about one of
// them has an unambiguous right answer. Vague test data makes a similarity
// test pass whatever the ranking does.
func corpus(t *testing.T) ([]Page, map[string]uuid.UUID) {
	t.Helper()
	ids := map[string]uuid.UUID{}
	mk := func(name, title, body, topic string, tags ...string) Page {
		id := uuid.New()
		ids[name] = id
		return Page{ID: id, Title: title, Body: body, Tags: tags, TopicName: topic, TopicColorKey: "a"}
	}
	return []Page{
		mk("rope", "Rope data structure",
			"A rope stores text as a balanced tree of substrings so splice and concat are logarithmic rather than linear.",
			"structures", "text", "trees"),
		mk("btree", "B-trees on disk",
			"A B-tree keeps keys sorted in wide nodes sized to a disk page so a lookup costs few seeks.",
			"structures", "trees", "storage"),
		mk("raft", "Raft consensus",
			"Raft elects a leader and replicates a log so a cluster agrees on an order of entries despite failures.",
			"distributed", "consensus"),
		mk("gossip", "Gossip protocols",
			"Gossip spreads membership by having each node tell a few random peers, converging without a leader.",
			"distributed", "consensus"),
	}, ids
}

func TestATypedQueryFindsTheSubjectItNames(t *testing.T) {
	pages, ids := corpus(t)
	got, stats, err := Near(pages, graphalgo.Graph{}, Query{Text: "balanced tree of substrings for text splicing", K: 2})
	if err != nil {
		t.Fatalf("Near: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("no neighbours for a query that names a page's own subject")
	}
	if got[0].PageID != ids["rope"].String() {
		t.Fatalf("nearest = %q, want the rope page", got[0].Title)
	}
	if len(stats.TopTerms) == 0 {
		t.Error("a typed query should report its own heaviest terms — it is the only account of why it ranked what it ranked")
	}
}

// The point of the whole feature: a typed query is not a page, so the two
// signals that need a page must come back absent rather than zero. Zero would
// read on screen as "shares nothing with you", which is a finding about the
// corpus rather than about the question.
func TestATypedQueryReportsTheSignalsItCannotHaveAsAbsent(t *testing.T) {
	pages, _ := corpus(t)
	got, stats, err := Near(pages, graphalgo.Graph{}, Query{Text: "raft leader election", K: 3})
	if err != nil {
		t.Fatalf("Near: %v", err)
	}
	if stats.HasOrigin {
		t.Error("HasOrigin is true for a query with no page")
	}
	for _, n := range got {
		if n.Hops != -1 {
			t.Errorf("%s: hops = %d, want -1 — a typed string has no node in the link graph", n.Title, n.Hops)
		}
		if n.SharedTags != 0 || n.TagJaccard != 0 {
			t.Errorf("%s: tag overlap reported against a query that has no tags", n.Title)
		}
	}
}

func TestAPageQueryStillCarriesItsOrigin(t *testing.T) {
	pages, ids := corpus(t)
	nodes := make([]graphalgo.NodeID, 0, len(pages))
	for _, p := range pages {
		nodes = append(nodes, graphalgo.NodeID(p.ID.String()))
	}
	links := graphalgo.Graph{
		Nodes: nodes,
		Edges: []graphalgo.Edge{{
			From: graphalgo.NodeID(ids["raft"].String()),
			To:   graphalgo.NodeID(ids["gossip"].String()),
		}},
	}
	got, stats, err := Near(pages, links, Query{PageID: ids["raft"].String(), K: 3})
	if err != nil {
		t.Fatalf("Near: %v", err)
	}
	if !stats.HasOrigin {
		t.Fatal("HasOrigin is false for a page query")
	}
	var gossip *Neighbour
	for i := range got {
		if got[i].PageID == ids["gossip"].String() {
			gossip = &got[i]
		}
	}
	if gossip == nil {
		t.Fatal("the linked, same-topic page is not among the neighbours")
	}
	if gossip.Hops != 1 {
		t.Errorf("hops = %d, want 1 — the two pages link to each other", gossip.Hops)
	}
	if gossip.SharedTags != 1 {
		t.Errorf("shared tags = %d, want 1 (consensus)", gossip.SharedTags)
	}
}

// Punctuation tokenizes to nothing. An empty vector is equidistant from every
// page, which the screen would draw as a ranking — so it has to be refused
// rather than answered.
func TestAQueryWithNoIndexableTermsIsRefusedRatherThanRanked(t *testing.T) {
	pages, _ := corpus(t)
	_, _, err := Near(pages, graphalgo.Graph{}, Query{Text: "... !!! ???", K: 3})
	if !errors.Is(err, ErrEmptyQuery) {
		t.Fatalf("err = %v, want ErrEmptyQuery", err)
	}
}

// Text wins when both are set — a client with a focused text box is asking
// about its contents, not about whatever row is still selected behind it.
func TestTextTakesPrecedenceOverASelectedPage(t *testing.T) {
	pages, ids := corpus(t)
	got, stats, err := Near(pages, graphalgo.Graph{}, Query{
		PageID: ids["raft"].String(),
		Text:   "balanced tree of substrings for text splicing",
		K:      1,
	})
	if err != nil {
		t.Fatalf("Near: %v", err)
	}
	if stats.HasOrigin {
		t.Error("the page id was treated as the origin even though text was given")
	}
	if got[0].PageID != ids["rope"].String() {
		t.Fatalf("nearest = %q, want the rope page — the text should have decided", got[0].Title)
	}
}
