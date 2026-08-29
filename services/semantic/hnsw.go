package semantic

import (
	"container/heap"
	"math"
	"sort"
)

// Index is a Hierarchical Navigable Small World graph over Vectors —
// approximate nearest neighbours in sublinear time (Malkov & Yashunin).
//
// THE IDEA, IN ONE PARAGRAPH. Every element lives on layer 0. Each element
// is also promoted to a random number of higher layers, with the probability
// of reaching layer l falling off exponentially — so the top layer holds a
// handful of elements spread across the whole space, and each layer down is
// denser. A search starts at the single entry point on the top layer, greedily
// walks to the closest element it can see, drops a layer, and repeats. The
// top layers are a coarse map that gets you to the right region in a few
// hops; layer 0 is where the answer is actually refined. That is the whole
// trick: a skip list, in a metric space.
//
// WHY IT IS HERE AT ALL. Brute force over this repo's corpus is trivially
// fast, and pretending otherwise would be dishonest. The index exists because
// § 09's claim is checkable: it reports how many distance computations it
// actually did against how many an exact scan would have done, AND it reports
// recall against the exact answer computed beside it. An approximate index
// that never shows you its recall is an index asking to be trusted; this one
// can be caught being wrong, on a corpus small enough that the exact answer
// is always affordable.
type Index struct {
	// M is how many neighbours an element keeps per layer above 0. The
	// classic knob: higher M is better recall and a bigger index.
	M int
	// Mmax0 is the same for layer 0, conventionally 2*M — layer 0 holds
	// every element, so it needs more connections to stay navigable.
	Mmax0 int
	// EfConstruction is how wide the candidate list is while inserting. It
	// buys build-time quality, not query time.
	EfConstruction int

	nodes  []node
	byID   map[string]int
	entry  int // index into nodes, -1 when empty
	maxLay int
	rng    *lcg
}

type node struct {
	id  string
	vec Vector
	// neighbours[l] is this node's adjacency on layer l. len() is its top
	// layer + 1.
	neighbours [][]int
}

// NewIndex builds an empty index. seed makes layer assignment deterministic:
// HNSW's level draw is random, and an index that reshuffles between two
// builds over identical input is one whose recall numbers cannot be compared
// between runs — which is the only thing this index is here to demonstrate.
func NewIndex(m, efConstruction int, seed uint64) *Index {
	return &Index{
		M:              m,
		Mmax0:          2 * m,
		EfConstruction: efConstruction,
		byID:           map[string]int{},
		entry:          -1,
		rng:            &lcg{state: seed | 1},
	}
}

// lcg is a small deterministic PRNG. math/rand would do, but seeding and
// reproducing a package-level source is a footgun this does not need — the
// level draw is the only randomness in the whole file.
type lcg struct{ state uint64 }

func (r *lcg) next() uint64 {
	r.state = r.state*6364136223846793005 + 1442695040888963407
	return r.state
}

func (r *lcg) float64() float64 {
	return float64(r.next()>>11) / float64(1<<53)
}

// randomLevel draws an element's top layer from the geometric distribution
// HNSW specifies: P(level >= l) = exp(-l / mL), with mL = 1/ln(M).
//
// That constant is not arbitrary. It is the value that makes the expected
// number of elements examined per layer roughly constant during descent,
// which is what makes the whole structure logarithmic rather than merely
// hierarchical.
func (ix *Index) randomLevel() int {
	mL := 1 / math.Log(float64(ix.M))
	l := int(-math.Log(1-ix.rng.float64()) * mL)
	// Cap the tower. Without it one unlucky draw creates twenty near-empty
	// layers that every later search has to walk through.
	if l > 16 {
		l = 16
	}
	return l
}

// dist is the metric. Cosine SIMILARITY is higher-is-better; every queue and
// comparison below wants lower-is-better, so the index works in 1 - cosine
// throughout and converts back only at the boundary.
func dist(a, b Vector) float64 { return 1 - Cosine(a, b) }

// Add inserts one element. Re-adding an existing id replaces its vector by
// rebuilding, not by patching: HNSW has no defined in-place update, and a
// half-updated neighbour list is worse than a slower correct one. The caller
// (a per-request rebuild at this repo's scale) never needs it.
func (ix *Index) Add(id string, v Vector) {
	if _, exists := ix.byID[id]; exists {
		return
	}
	level := ix.randomLevel()
	n := node{id: id, vec: v, neighbours: make([][]int, level+1)}
	idx := len(ix.nodes)
	ix.nodes = append(ix.nodes, n)
	ix.byID[id] = idx

	if ix.entry == -1 {
		ix.entry = idx
		ix.maxLay = level
		return
	}

	// Phase 1: greedy descent from the top down to level+1, one neighbour at
	// a time. No candidate list here — above the element's own top layer we
	// only need a good entry point, not a good result set.
	cur := ix.entry
	for l := ix.maxLay; l > level; l-- {
		cur = ix.greedyStep(v, cur, l, nil)
	}

	// Phase 2: at every layer the element belongs to, search properly and
	// connect. Connections are bidirectional, and the reverse side is pruned
	// back to its own budget — an element that becomes everyone's neighbour
	// is a hub that destroys navigability.
	for l := min(level, ix.maxLay); l >= 0; l-- {
		candidates := ix.searchLayer(v, []int{cur}, ix.EfConstruction, l, nil, nil)
		selected := ix.selectNeighbours(v, candidates, ix.maxConn(l))
		ix.nodes[idx].neighbours[l] = selected
		for _, s := range selected {
			ix.nodes[s].neighbours[l] = append(ix.nodes[s].neighbours[l], idx)
			if len(ix.nodes[s].neighbours[l]) > ix.maxConn(l) {
				ix.nodes[s].neighbours[l] = ix.selectNeighbours(
					ix.nodes[s].vec, ix.nodes[s].neighbours[l], ix.maxConn(l))
			}
		}
		if len(candidates) > 0 {
			cur = candidates[0]
		}
	}

	if level > ix.maxLay {
		ix.maxLay = level
		ix.entry = idx
	}
}

func (ix *Index) maxConn(layer int) int {
	if layer == 0 {
		return ix.Mmax0
	}
	return ix.M
}

// greedyStep walks downhill on one layer until no neighbour is closer. This
// is the coarse half of the descent — it commits to the first improvement it
// finds and never backtracks, which is exactly why it is cheap and exactly
// why it needs the layers to be right about which region to be in.
func (ix *Index) greedyStep(q Vector, start, layer int, stats *SearchStats) int {
	cur := start
	curDist := dist(q, ix.nodes[cur].vec)
	count(stats, 1, 1)
	for {
		improved := false
		for _, nb := range ix.neighboursAt(cur, layer) {
			count(stats, 1, 0)
			if d := dist(q, ix.nodes[nb].vec); d < curDist {
				cur, curDist, improved = nb, d, true
			}
		}
		if !improved {
			return cur
		}
		count(stats, 0, 1)
	}
}

// count is the one place the instrumentation touches the algorithm. A nil
// stats pointer means "not measuring" — insertion never is, because the
// numbers § 09 reports are about a QUERY, and folding build cost into them
// would make the comparison against a brute-force scan meaningless.
func count(stats *SearchStats, comparisons, hops int) {
	if stats == nil {
		return
	}
	stats.Comparisons += comparisons
	stats.Hops += hops
}

func (ix *Index) neighboursAt(n, layer int) []int {
	if layer >= len(ix.nodes[n].neighbours) {
		return nil
	}
	return ix.nodes[n].neighbours[layer]
}

// searchLayer is HNSW's real search: a best-first expansion holding the ef
// closest elements found so far, stopping when the nearest unexplored
// candidate is further than the worst kept result.
//
// `allow` is the filter, and it is applied HERE rather than to the returned
// list. That placement is the whole point: post-filtering asks for k=5,
// throws three away and ships two, and recall collapses exactly when the
// filter is narrow — which is when someone is relying on it. Filtering during
// the descent means the traversal still walks THROUGH excluded elements (they
// are the roads) but never keeps them, so the result set is k elements that
// pass the filter, or every one that exists.
func (ix *Index) searchLayer(q Vector, entries []int, ef, layer int, allow func(string) bool, stats *SearchStats) []int {
	visited := make(map[int]bool, ef*2)
	// candidates: min-heap by distance — where to expand next.
	// results: max-heap by distance — the ef best kept so far, worst on top.
	candidates := &pq{}
	results := &pq{max: true}
	heap.Init(candidates)
	heap.Init(results)

	for _, e := range entries {
		if visited[e] {
			continue
		}
		visited[e] = true
		d := dist(q, ix.nodes[e].vec)
		count(stats, 1, 1)
		heap.Push(candidates, item{e, d})
		if allow == nil || allow(ix.nodes[e].id) {
			heap.Push(results, item{e, d})
		}
	}

	for candidates.Len() > 0 {
		c := heap.Pop(candidates).(item)
		// The stopping rule: if the closest thing left to explore is further
		// than the worst thing already kept, nothing reachable from here can
		// improve the answer.
		if results.Len() >= ef && c.d > results.items[0].d {
			break
		}
		for _, nb := range ix.neighboursAt(c.n, layer) {
			if visited[nb] {
				continue
			}
			visited[nb] = true
			d := dist(q, ix.nodes[nb].vec)
			count(stats, 1, 1)
			if results.Len() < ef || d < results.items[0].d {
				heap.Push(candidates, item{nb, d})
				if allow == nil || allow(ix.nodes[nb].id) {
					heap.Push(results, item{nb, d})
					if results.Len() > ef {
						heap.Pop(results)
					}
				}
			}
		}
	}

	out := make([]item, 0, results.Len())
	for results.Len() > 0 {
		out = append(out, heap.Pop(results).(item))
	}
	// Popped worst-first from a max-heap, so reverse into nearest-first.
	sort.Slice(out, func(i, j int) bool {
		if out[i].d != out[j].d {
			return out[i].d < out[j].d
		}
		return ix.nodes[out[i].n].id < ix.nodes[out[j].n].id
	})
	ids := make([]int, len(out))
	for i, it := range out {
		ids[i] = it.n
	}
	return ids
}

// selectNeighbours is the HEURISTIC pruning rule, not plain "keep the M
// closest".
//
// The difference matters and is the single thing most naive HNSW
// implementations get wrong. Keeping the M closest fills a node's neighbour
// list with elements from one dense cluster, and the graph loses every long
// edge — which is what made it navigable. The heuristic keeps a candidate
// only if it is closer to the query than to any neighbour already kept, so a
// distant element in an otherwise unrepresented direction wins a slot over a
// nearer one that duplicates a direction already covered.
func (ix *Index) selectNeighbours(q Vector, candidates []int, m int) []int {
	type scored struct {
		n int
		d float64
	}
	sorted := make([]scored, 0, len(candidates))
	seen := map[int]bool{}
	for _, c := range candidates {
		if seen[c] {
			continue
		}
		seen[c] = true
		sorted = append(sorted, scored{c, dist(q, ix.nodes[c].vec)})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].d != sorted[j].d {
			return sorted[i].d < sorted[j].d
		}
		return ix.nodes[sorted[i].n].id < ix.nodes[sorted[j].n].id
	})

	out := make([]int, 0, m)
	for _, c := range sorted {
		if len(out) >= m {
			break
		}
		keep := true
		for _, k := range out {
			if dist(ix.nodes[c.n].vec, ix.nodes[k].vec) < c.d {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, c.n)
		}
	}
	// The heuristic can under-fill when every candidate is mutually close.
	// Top up by distance rather than leaving a node under-connected, which
	// would strand it.
	for _, c := range sorted {
		if len(out) >= m {
			break
		}
		already := false
		for _, k := range out {
			if k == c.n {
				already = true
				break
			}
		}
		if !already {
			out = append(out, c.n)
		}
	}
	return out
}

// Result is one neighbour and how close it is, as a SIMILARITY (higher is
// better) — the index works in distance internally and converts once, here,
// so no caller has to remember which direction the number runs.
type Result struct {
	ID         string  `json:"id"`
	Similarity float64 `json:"similarity"`
}

// SearchStats is what makes this index arguable rather than merely fast.
type SearchStats struct {
	// Comparisons is how many distance computations the approximate search
	// performed. Compared against ExactComparisons, this is the entire
	// justification for the structure existing.
	Comparisons int `json:"comparisons"`
	// ExactComparisons is what a brute-force scan would have cost.
	ExactComparisons int `json:"exact_comparisons"`
	// Hops is how many nodes the descent visited across all layers.
	Hops int `json:"hops"`
	// Layers is the height of the tower actually built.
	Layers int `json:"layers"`
	// RecallAtK is how many of the exact top-k the approximate search found,
	// over k. 1.0 means it missed nothing. Computed by running the exact
	// scan beside every query — speed without recall is half a result.
	RecallAtK float64 `json:"recall_at_k"`
	// Candidates is how many elements passed the filter at all.
	Candidates int `json:"candidates"`
}

// Search returns the k nearest elements to q, excluding `exclude` (a page is
// never its own neighbour), restricted to elements `allow` accepts.
//
// ef is the search-time candidate width. It must be at least k; a caller
// asking for 5 results with ef=1 is asking for a greedy walk, not a search.
func (ix *Index) Search(q Vector, k, ef int, exclude string, allow func(string) bool) ([]Result, SearchStats) {
	stats := SearchStats{ExactComparisons: len(ix.nodes), Layers: ix.maxLay + 1}
	if ix.entry == -1 || k <= 0 {
		return nil, stats
	}
	if ef < k {
		ef = k
	}

	filter := func(id string) bool {
		if id == exclude {
			return false
		}
		return allow == nil || allow(id)
	}
	for _, n := range ix.nodes {
		if filter(n.id) {
			stats.Candidates++
		}
	}

	cur := ix.entry
	for l := ix.maxLay; l > 0; l-- {
		cur = ix.greedyStep(q, cur, l, &stats)
	}
	found := ix.searchLayer(q, []int{cur}, ef, 0, filter, &stats)

	if len(found) > k {
		found = found[:k]
	}
	out := make([]Result, len(found))
	for i, n := range found {
		out[i] = Result{ID: ix.nodes[n].id, Similarity: Cosine(q, ix.nodes[n].vec)}
	}

	// Recall against the exact answer, every query. The exact scan is O(N)
	// and this corpus is small; when it stops being affordable, this becomes
	// a sampled check rather than a removed one.
	exact := ix.Exact(q, k, exclude, allow)
	stats.RecallAtK = recall(out, exact)
	return out, stats
}

// Exact is the brute-force answer — every element scored, sorted. It exists
// to be compared against, not to be fast.
func (ix *Index) Exact(q Vector, k int, exclude string, allow func(string) bool) []Result {
	out := make([]Result, 0, len(ix.nodes))
	for _, n := range ix.nodes {
		if n.id == exclude {
			continue
		}
		if allow != nil && !allow(n.id) {
			continue
		}
		out = append(out, Result{ID: n.id, Similarity: Cosine(q, n.vec)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Similarity != out[j].Similarity {
			return out[i].Similarity > out[j].Similarity
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > k {
		out = out[:k]
	}
	return out
}

// recall is |approx ∩ exact| / |exact|. Zero exact results is recall 1: the
// search correctly found everything there was, and reporting 0 there would
// read as a failure of the index rather than an empty corpus.
func recall(approx, exact []Result) float64 {
	if len(exact) == 0 {
		return 1
	}
	want := make(map[string]bool, len(exact))
	for _, e := range exact {
		want[e.ID] = true
	}
	hit := 0
	for _, a := range approx {
		if want[a.ID] {
			hit++
		}
	}
	return float64(hit) / float64(len(exact))
}

// Len is how many elements the index holds.
func (ix *Index) Len() int { return len(ix.nodes) }

// LayerSizes is how many elements reached each layer — the tower's own
// shape, which § 09 draws. Layer 0 always holds everything.
func (ix *Index) LayerSizes() []int {
	out := make([]int, ix.maxLay+1)
	for _, n := range ix.nodes {
		for l := 0; l < len(n.neighbours); l++ {
			out[l]++
		}
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// item is one element in a search frontier: a node and its distance to the
// query.
type item struct {
	n int
	d float64
}

// pq is both queues HNSW's layer search needs — a min-heap of candidates to
// expand and a max-heap of results to keep — differing only in comparison
// direction. One type with a flag rather than two, because they are otherwise
// identical and a second copy is a second place for the tie rule to drift.
//
// Ties break on node index, so a query over an unchanged index returns the
// same list every time. Without it the answer depends on heap internals,
// which is the kind of nondeterminism that makes a recall number unarguable
// in the wrong direction.
type pq struct {
	items []item
	max   bool
}

func (p pq) Len() int { return len(p.items) }

func (p pq) Less(i, j int) bool {
	if p.items[i].d != p.items[j].d {
		if p.max {
			return p.items[i].d > p.items[j].d
		}
		return p.items[i].d < p.items[j].d
	}
	return p.items[i].n < p.items[j].n
}

func (p pq) Swap(i, j int) { p.items[i], p.items[j] = p.items[j], p.items[i] }

func (p *pq) Push(x any) { p.items = append(p.items, x.(item)) }

func (p *pq) Pop() any {
	old := p.items
	n := len(old)
	it := old[n-1]
	p.items = old[:n-1]
	return it
}
