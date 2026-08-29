package discoverrest

import documentv1 "marginal/document-service/genproto/documentv1"

type neighbourJSON struct {
	PageID        string   `json:"page_id"`
	Title         string   `json:"title"`
	Excerpt       string   `json:"excerpt"`
	TopicName     string   `json:"topic_name"`
	TopicColorKey string   `json:"topic_color_key"`
	Tags          []string `json:"tags"`
	// The three signals, kept separate all the way to the browser. Blending
	// them into one number server-side would make the screen undebuggable in
	// exactly the case worth debugging.
	Cosine     float64 `json:"cosine"`
	SharedTags int32   `json:"shared_tags"`
	TagJaccard float64 `json:"tag_jaccard"`
	// -1 is unreachable, not "very far".
	Hops int32 `json:"hops"`
}

type statsJSON struct {
	Comparisons      int32    `json:"comparisons"`
	ExactComparisons int32    `json:"exact_comparisons"`
	Hops             int32    `json:"hops"`
	Layers           int32    `json:"layers"`
	RecallAtK        float64  `json:"recall_at_k"`
	Candidates       int32    `json:"candidates"`
	LayerSizes       []int32  `json:"layer_sizes"`
	Corpus           int32    `json:"corpus"`
	TopTerms         []string `json:"top_terms"`
	M                int32    `json:"m"`
	EfSearch         int32    `json:"ef_search"`
	Dimensions       int32    `json:"dimensions"`
}

type nearJSON struct {
	Neighbours []neighbourJSON `json:"neighbours"`
	Stats      statsJSON       `json:"stats"`
	Topics     []string        `json:"topics"`
}

func toNearJSON(r *documentv1.NearResponse) nearJSON {
	out := nearJSON{
		Neighbours: make([]neighbourJSON, 0, len(r.GetNeighbours())),
		Topics:     emptyStrings(r.GetTopics()),
	}
	for _, n := range r.GetNeighbours() {
		out.Neighbours = append(out.Neighbours, neighbourJSON{
			PageID:        n.GetPageId(),
			Title:         n.GetTitle(),
			Excerpt:       n.GetExcerpt(),
			TopicName:     n.GetTopicName(),
			TopicColorKey: n.GetTopicColorKey(),
			Tags:          emptyStrings(n.GetTags()),
			Cosine:        n.GetCosine(),
			SharedTags:    n.GetSharedTags(),
			TagJaccard:    n.GetTagJaccard(),
			Hops:          n.GetHops(),
		})
	}
	s := r.GetStats()
	sizes := s.GetLayerSizes()
	if sizes == nil {
		sizes = []int32{}
	}
	out.Stats = statsJSON{
		Comparisons:      s.GetComparisons(),
		ExactComparisons: s.GetExactComparisons(),
		Hops:             s.GetHops(),
		Layers:           s.GetLayers(),
		RecallAtK:        s.GetRecallAtK(),
		Candidates:       s.GetCandidates(),
		LayerSizes:       sizes,
		Corpus:           s.GetCorpus(),
		TopTerms:         emptyStrings(s.GetTopTerms()),
		M:                s.GetM(),
		EfSearch:         s.GetEfSearch(),
		Dimensions:       s.GetDimensions(),
	}
	return out
}

// emptyStrings makes an absent repeated field `[]` rather than `null`, so a
// client can iterate without a guard — the same rule every other converter
// in this package follows.
func emptyStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
