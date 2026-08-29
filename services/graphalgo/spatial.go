package graphalgo

import "sort"

// NeighbourMajority answers a question the link graph cannot: what is the
// label of the REGION a node sits in, as opposed to the label the node
// declares for itself.
//
// § 07 GRAPH offers three ways to colour the same nodes, and the whole point
// of offering three is that they can disagree:
//
//   - TOPIC   — what the page says it is about. Declared, a column.
//   - CLUSTER — which connected component it is in. Emergent from citation.
//   - SPACE   — what its spatial NEIGHBOURS are about. Emergent from where
//     the force layout dragged it, which is a different emergence again:
//     two pages can be spatially adjacent without either linking to the
//     other, because both are pulled by the same third page.
//
// This computes the third. `adjacent` is the Delaunay dual of the Voronoi
// diagram — "these two cells share a border" — and `label` is whatever the
// caller is voting on (a topic colour key, in practice). A node takes the
// label held by most of its spatial neighbours, INCLUDING ITSELF: a page
// with one neighbour of another topic should not flip on a vote of one, and
// counting itself makes the tie rule fall out rather than needing a clause.
//
// A tie goes to the node's OWN label, and only then to the label string.
// Both halves matter: a page whose neighbourhood is evenly split is a page
// its neighbours say nothing about, so overruling its declared topic there
// would manufacture a disagreement out of no evidence; and the alphabetical
// fallback keeps the result deterministic, because a hue that changes
// between two runs over identical input is a hue nobody can read a finding
// off.
//
// Nodes with no label of their own and no labelled neighbour are absent from
// the result rather than mapped to "": untopiced is a real state the caller
// draws differently, and inventing a majority for a node that has no
// evidence either way is exactly the lie this screen exists to avoid.
func NeighbourMajority(adjacent []DelaunayPair, label map[NodeID]string) map[NodeID]string {
	neighbours := make(map[NodeID][]NodeID, len(label))
	for _, p := range adjacent {
		neighbours[p.A] = append(neighbours[p.A], p.B)
		neighbours[p.B] = append(neighbours[p.B], p.A)
	}

	out := make(map[NodeID]string, len(label))
	for node := range label {
		own := label[node]
		counts := map[string]int{}
		if own != "" {
			counts[own]++
		}
		for _, nb := range neighbours[node] {
			if l := label[nb]; l != "" {
				counts[l]++
			}
		}
		if len(counts) == 0 {
			continue
		}
		best := make([]string, 0, len(counts))
		for l := range counts {
			best = append(best, l)
		}
		sort.Slice(best, func(i, j int) bool {
			if counts[best[i]] != counts[best[j]] {
				return counts[best[i]] > counts[best[j]]
			}
			if (best[i] == own) != (best[j] == own) {
				return best[i] == own
			}
			return best[i] < best[j]
		})
		out[node] = best[0]
	}
	return out
}
