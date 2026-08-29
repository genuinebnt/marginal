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
	// TopTerms are the query page's own heaviest terms — why it sits where it
	// sits. A similarity nobody can interrogate is one nobody should trust.
	TopTerms []string
}

// ErrUnknownPage is returned when the requested page is not in the live
// corpus — deleted, or never existed. Distinguished from "no neighbours"
// because they call for different words on screen.
var ErrUnknownPage = errors.New("discover: page is not in the live corpus")

// Query is one Near request.
type Query struct {
	PageID string
	K      int
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

	self, ok := byID[q.PageID]
	if !ok {
		return nil, Stats{Corpus: len(pages)}, ErrUnknownPage
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

	hits, searchStats := ix.Search(vectors[self], q.K, ParamEfSearch, q.PageID, allow)

	// Hop distance, from the same BFS /graph/neighborhood runs. A second
	// signal over a different structure — not a re-scoring of the first.
	dist, _ := graphalgo.BFS(links, graphalgo.NodeID(q.PageID))

	own := pages[self]
	out := make([]Neighbour, 0, len(hits))
	for _, h := range hits {
		p := pages[byID[h.ID]]
		shared, jaccard := tagOverlap(own.Tags, p.Tags)
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
		TopTerms:    corpus.TopTerms(terms[self], 8),
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
