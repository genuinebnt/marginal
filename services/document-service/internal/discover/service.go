package discover

import (
	"errors"
	"sort"
	"strings"

	"marginal/graphalgo"
	"marginal/semantic"
)

// IndexParams are the HNSW knobs § 09's INDEX panel reports. Constants rather
// than configuration: they are shown on screen precisely so the numbers
// beside them (recall, comparisons) can be read against a stated setting, and
// a setting that moves per deployment is one the screen cannot state.
const (
	ParamM              = 16
	ParamEfConstruction = 64
	ParamEfSearch       = 64
	// Seed makes the level draw reproducible. Two builds over the same corpus
	// must give the same answer, or the recall figure printed on the screen
	// cannot be compared with the one printed a minute ago.
	ParamSeed = 0x5EED
)

// Neighbour is one result, with its three signals kept SEPARATE.
//
// They are never blended into a single score, and that is a decision rather
// than an omission. A blended number is unarguable: when it puts the wrong
// page first there is nothing to inspect, because the one thing you can see
// is the output of the blend. Three numbers can be read against each other,
// and the interesting row is always the one where they disagree — high
// cosine, no shared tags, unreachable by link is prose similarity finding
// something the graph and the tags both missed, which is the only reason to
// run an embedding index at all.
type Neighbour struct {
	PageID string
	Title  string
	// Excerpt is the first ~120 characters of the indexed body — the same
	// string the vector was built from, so what you read is what was scored.
	Excerpt       string
	TopicName     string
	TopicColorKey string
	Tags          []string

	// Cosine similarity over the hashed TF-IDF vectors, in [0,1].
	Cosine float64
	// SharedTags is |A ∩ B| and TagJaccard is |A ∩ B| / |A ∪ B|. Both, not
	// one: the count is what a person reads, the ratio is what compares
	// fairly between a page with two tags and a page with nine.
	SharedTags int
	TagJaccard float64
	// Hops is undirected link distance. -1 means unreachable — deliberately
	// not a large number, because "far" and "not connected at all" are
	// different findings and averaging over a sentinel silently merges them.
	Hops int
}

// Stats is everything § 09 needs to be checkable.
type Stats struct {
	semantic.SearchStats
	// LayerSizes is the HNSW tower's own shape, largest (layer 0) first.
	LayerSizes []int
	// Corpus is how many live pages were indexed.
	Corpus int
	// TopTerms are the query's own heaviest terms — the page's, or the typed
	// string's. Why it sits where it sits: a similarity nobody can
	// interrogate is one nobody should trust.
	TopTerms []string
	// HasOrigin is false for a typed query — which has no tags and no node in
	// the link graph, so SharedTags/TagJaccard/Hops on every result below are
	// not small numbers but absent ones, and the screen has to say so.
	HasOrigin bool
}

// ErrUnknownPage is returned when the requested page is not in the live
// corpus — deleted, or never existed. Distinguished from "no neighbours"
// because they call for different words on screen.
var ErrUnknownPage = errors.New("discover: page is not in the live corpus")

// ErrEmptyQuery is returned when a typed query holds nothing the tokenizer
// keeps — punctuation, or stop words alone. Distinguished from "no results"
// for the same reason as above: one is a corpus finding, the other is that
// there was no question.
var ErrEmptyQuery = errors.New("discover: query has no indexable terms")

// Query is one Near request.
//
// Exactly one of PageID and Text is the origin. PageID asks "what is near
// this page"; Text asks "what is near this sentence", vectorising the typed
// string through the same corpus IDF the pages were embedded with — the same
// index, the same descent, a query vector that simply did not come from a
// row. Setting both is a caller error and Text wins, because a screen that
// has a text box focused is asking about the text in it.
type Query struct {
	PageID string
	// Text, when non-empty, replaces PageID as the origin.
	Text string
	K    int
	// Topics restricts results to these topic names. Empty means every topic.
	Topics []string
	// MustTags requires every listed tag. Empty means no tag constraint.
	MustTags []string
}

// Near is the whole feature.
//
// The filter is passed INTO the search rather than applied to its output.
// Post-filtering asks for k=5, throws three away and ships two, and recall
// collapses exactly when the filter is narrow — which is when someone is
// relying on it. semantic.Index applies it during the greedy descent instead:
// excluded elements are still walked through (they are the roads) and never
// kept.
func Near(pages []Page, links graphalgo.Graph, q Query) ([]Neighbour, Stats, error) {
	if q.K <= 0 {
		q.K = 5
	}

	docs := make([]semantic.Document, len(pages))
	terms := make([][]string, len(pages))
	byID := make(map[string]int, len(pages))
	for i, p := range pages {
		// Title and body together. A title is short and carries the page's
		// most load-bearing words; indexing only the body loses exactly the
		// pages whose argument is in their name.
		terms[i] = semantic.Tokenize(p.Title + " " + p.Body)
		docs[i] = semantic.Document{ID: p.ID.String(), Terms: terms[i]}
		byID[p.ID.String()] = i
	}
	corpus := semantic.NewCorpus(docs)

	ix := semantic.NewIndex(ParamM, ParamEfConstruction, ParamSeed)
	vectors := make([]semantic.Vector, len(pages))
	for i, p := range pages {
		vectors[i] = corpus.Embed(terms[i])
		ix.Add(p.ID.String(), vectors[i])
	}

	// The origin vector, and what the origin can be compared against.
	//
	// A typed query is not a page: it has no tags and no node in the link
	// graph, so two of the three signals have no question to answer for it.
	// They are reported ABSENT rather than zero — "shares no tags with you"
	// and "you have no tags" are different findings, and a screen that
	// printed 0 for both would be making the second look like the first.
	var (
		qv        semantic.Vector
		qTerms    []string
		ownTags   []string
		exclude   string
		hasOrigin bool
	)
	if strings.TrimSpace(q.Text) != "" {
		qTerms = semantic.Tokenize(q.Text)
		qv = corpus.Embed(qTerms)
		if len(qTerms) == 0 {
			// Punctuation only, or nothing the tokenizer keeps. An empty
			// vector matches everything equally badly, which reads on screen
			// as a ranking; say there is nothing to rank instead.
			return nil, Stats{Corpus: len(pages), LayerSizes: ix.LayerSizes()}, ErrEmptyQuery
		}
	} else {
		self, ok := byID[q.PageID]
		if !ok {
			return nil, Stats{Corpus: len(pages)}, ErrUnknownPage
		}
		qv, qTerms = vectors[self], terms[self]
		ownTags, exclude, hasOrigin = pages[self].Tags, q.PageID, true
	}

	wantTopic := map[string]bool{}
	for _, t := range q.Topics {
		wantTopic[strings.ToLower(t)] = true
	}
	allow := func(id string) bool {
		i, ok := byID[id]
		if !ok {
			return false
		}
		p := pages[i]
		if len(wantTopic) > 0 && !wantTopic[strings.ToLower(p.TopicName)] {
			return false
		}
		for _, need := range q.MustTags {
			if !contains(p.Tags, need) {
				return false
			}
		}
		return true
	}

	hits, searchStats := ix.Search(qv, q.K, ParamEfSearch, exclude, allow)

	// Hop distance, from the same BFS /graph/neighborhood runs. A second
	// signal over a different structure — not a re-scoring of the first.
	// A typed query has no node to start from, so there is no BFS to run and
	// every hop stays at the -1 the loop below defaults it to.
	var dist map[graphalgo.NodeID]int
	if hasOrigin {
		dist, _ = graphalgo.BFS(links, graphalgo.NodeID(q.PageID))
	}

	out := make([]Neighbour, 0, len(hits))
	for _, h := range hits {
		p := pages[byID[h.ID]]
		shared, jaccard := tagOverlap(ownTags, p.Tags)
		hops := -1
		if d, ok := dist[graphalgo.NodeID(h.ID)]; ok {
			hops = d
		}
		out = append(out, Neighbour{
			PageID:        h.ID,
			Title:         p.Title,
			Excerpt:       excerpt(p.Body, 120),
			TopicName:     p.TopicName,
			TopicColorKey: p.TopicColorKey,
			Tags:          p.Tags,
			Cosine:        h.Similarity,
			SharedTags:    shared,
			TagJaccard:    jaccard,
			Hops:          hops,
		})
	}

	return out, Stats{
		SearchStats: searchStats,
		LayerSizes:  ix.LayerSizes(),
		Corpus:      len(pages),
		TopTerms:    corpus.TopTerms(qTerms, 8),
		HasOrigin:   hasOrigin,
	}, nil
}

// tagOverlap returns both the count and the ratio. The count is what a person
// reads off the screen; the ratio is what compares fairly between a page
// carrying two tags and one carrying nine.
func tagOverlap(a, b []string) (int, float64) {
	set := make(map[string]bool, len(a))
	for _, t := range a {
		set[t] = true
	}
	shared := 0
	union := len(a)
	for _, t := range b {
		if set[t] {
			shared++
		} else {
			union++
		}
	}
	if union == 0 {
		return 0, 0
	}
	return shared, float64(shared) / float64(union)
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// excerpt trims to n runes on a word boundary. Runes, not bytes: cutting a
// multi-byte character in half produces a replacement glyph on screen.
func excerpt(body string, n int) string {
	body = strings.Join(strings.Fields(body), " ")
	r := []rune(body)
	if len(r) <= n {
		return body
	}
	cut := string(r[:n])
	if i := strings.LastIndex(cut, " "); i > n/2 {
		cut = cut[:i]
	}
	return cut + "…"
}

// SortedTopics is every distinct topic in the corpus, for the scope control.
func SortedTopics(pages []Page) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, p := range pages {
		if p.TopicName == "" || seen[p.TopicName] {
			continue
		}
		seen[p.TopicName] = true
		out = append(out, p.TopicName)
	}
	sort.Strings(out)
	return out
}
