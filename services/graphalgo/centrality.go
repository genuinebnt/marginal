package graphalgo

import "sort"

// Betweenness centrality and modularity — the two numbers § 07 GRAPH reports
// about a node and about a partition, and the two that were placeholders in
// the UI until this file existed.

// Betweenness returns each node's betweenness centrality, by Brandes'
// algorithm: O(VE) on an unweighted graph, against O(V³) for the naive
// all-pairs-then-count approach.
//
// The measure answers "how much traffic would route through this page if
// every pair of pages took a shortest path". A page can have few links and
// high betweenness — that is precisely the interesting case, because it is a
// bridge, and deleting it disconnects things its degree never warned you
// about.
//
// Computed over the UNDIRECTED view, deliberately. A [[link]] is a claim that
// two pages are related, and relatedness is not one-way: treating it as
// directed would give every page that is linked-to-but-never-links a
// betweenness of zero, which says something about link direction rather than
// about the page's role.
//
// Values are normalised to [0,1] by the number of ordered pairs, so they
// compare across graphs of different sizes.
func Betweenness(g Graph) map[NodeID]float64 {
	adj := buildAdjacency(g)
	bc := make(map[NodeID]float64, len(g.Nodes))
	for _, n := range g.Nodes {
		bc[n] = 0
	}

	for _, s := range g.Nodes {
		// Single-source shortest paths, counting how many there are.
		var stack []NodeID
		pred := make(map[NodeID][]NodeID, len(g.Nodes))
		sigma := make(map[NodeID]float64, len(g.Nodes)) // shortest paths s->v
		dist := make(map[NodeID]int, len(g.Nodes))
		for _, n := range g.Nodes {
			dist[n] = -1
		}
		sigma[s] = 1
		dist[s] = 0

		queue := []NodeID{s}
		for len(queue) > 0 {
			v := queue[0]
			queue = queue[1:]
			stack = append(stack, v)
			for _, w := range adj.undirected[v] {
				if dist[w] < 0 { // first time seen: one level deeper
					dist[w] = dist[v] + 1
					queue = append(queue, w)
				}
				if dist[w] == dist[v]+1 { // another shortest path to w through v
					sigma[w] += sigma[v]
					pred[w] = append(pred[w], v)
				}
			}
		}

		// Accumulate dependencies back-to-front. Walking the stack in reverse
		// visits every node after all of its successors, which is what makes
		// one pass sufficient — the insight the whole algorithm rests on.
		delta := make(map[NodeID]float64, len(g.Nodes))
		for i := len(stack) - 1; i >= 0; i-- {
			w := stack[i]
			for _, v := range pred[w] {
				if sigma[w] != 0 {
					delta[v] += (sigma[v] / sigma[w]) * (1 + delta[w])
				}
			}
			if w != s {
				bc[w] += delta[w]
			}
		}
	}

	// Each pair is counted from both ends on an undirected graph, and the
	// normaliser is the number of pairs excluding the node itself.
	n := float64(len(g.Nodes))
	if n > 2 {
		scale := 2 / ((n - 1) * (n - 2))
		for k := range bc {
			bc[k] *= scale
		}
	}
	return bc
}

// Modularity is Newman's Q for a given partition: the fraction of edges
// falling inside communities, minus what that fraction would be if the same
// degree sequence were wired at random.
//
//	Q = Σ_c [ e_c/m − (d_c/2m)² ]
//
// Q near 0 means the partition explains nothing the degrees do not already;
// 0.3–0.7 is the usual range for a partition that means something.
//
// The screen computes it twice — once by TOPIC (declared) and once by
// CLUSTER (emergent) — and the gap between them is the finding: if topics
// scored much lower, the topics would be describing something other than how
// the pages actually link.
//
// Nodes with no community ("" group) are excluded from both terms rather than
// pooled into one: an untopiced page is not a community, and treating it as
// one would inflate Q with a group that means "we do not know".
func Modularity(g Graph, community map[NodeID]string) float64 {
	adj := buildAdjacency(g)

	// m counts undirected edges once. Self-loops and duplicate [[links]]
	// between the same pair are counted as they appear — the graph is what it
	// is, and silently deduping here would make Q disagree with the edge
	// count the UI shows beside it.
	m := float64(len(g.Edges))
	if m == 0 {
		return 0
	}

	inside := map[string]float64{} // edges with both ends in c
	degree := map[string]float64{} // summed degree of c's nodes

	for _, n := range g.Nodes {
		c, ok := community[n]
		if !ok || c == "" {
			continue
		}
		degree[c] += float64(len(adj.undirected[n]))
	}
	for _, e := range g.Edges {
		a, aok := community[e.From]
		b, bok := community[e.To]
		if !aok || !bok || a == "" || b == "" || a != b {
			continue
		}
		inside[a]++
	}

	var q float64
	for _, e := range inside {
		q += e / m
	}
	for _, d := range degree {
		frac := d / (2 * m)
		q -= frac * frac
	}
	return q
}

// TopBetweenness returns the n nodes with the highest betweenness, highest
// first, breaking ties by id so the order is stable across calls — an
// unstable "top bridges" list reorders itself on every refresh for no reason
// the reader can see.
func TopBetweenness(bc map[NodeID]float64, n int) []NodeID {
	ids := make([]NodeID, 0, len(bc))
	for id := range bc {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if bc[ids[i]] != bc[ids[j]] {
			return bc[ids[i]] > bc[ids[j]]
		}
		return ids[i] < ids[j]
	})
	if n < len(ids) {
		ids = ids[:n]
	}
	return ids
}
