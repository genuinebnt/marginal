package graphalgo

// BFS computes shortest-path distances (in link hops) and each reached
// node's predecessor on some shortest path from source, over the
// UNDIRECTED graph — "link distance" doesn't care which way a link
// points, only that one exists. dist[source] == 0; a node absent from
// dist is unreachable from source.
//
// Grouping this same result by distance value (0, 1, 2, ...) and
// revealing one group at a time is exactly graph-algorithms.html's "BFS
// rendered as an animated wavefront, frontier widths per level" — no
// separate algorithm needed for it: the frontier at level k is just
// {n : dist[n] == k}.
func BFS(g Graph, source NodeID) (dist map[NodeID]int, prev map[NodeID]NodeID) {
	adj := buildAdjacency(g)
	dist = map[NodeID]int{source: 0}
	prev = map[NodeID]NodeID{}
	queue := []NodeID{source}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, nb := range adj.undirected[cur] {
			if _, seen := dist[nb]; !seen {
				dist[nb] = dist[cur] + 1
				prev[nb] = cur
				queue = append(queue, nb)
			}
		}
	}
	return dist, prev
}

// ShortestPath reconstructs one shortest path from source to target
// (inclusive of both ends) from BFS's own dist/prev maps. Returns
// (nil, false) if target is unreachable from source.
func ShortestPath(dist map[NodeID]int, prev map[NodeID]NodeID, source, target NodeID) ([]NodeID, bool) {
	if _, ok := dist[target]; !ok {
		return nil, false
	}
	path := []NodeID{target}
	for path[len(path)-1] != source {
		cur := path[len(path)-1]
		p, ok := prev[cur]
		if !ok {
			break // cur == source already, or a broken chain that shouldn't happen given dist has target
		}
		path = append(path, p)
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path, true
}

// ForwardReachable is every node reachable from source following only
// OUTBOUND links, with hop-distance — "blast radius": what a cascading
// delete starting at source would actually take with it, since walking
// forward through [[link]]s is exactly what "downstream of this page"
// means. Directed, unlike BFS above, on purpose: a page that links TO
// source is not something deleting source would ever touch.
func ForwardReachable(g Graph, source NodeID) map[NodeID]int {
	adj := buildAdjacency(g)
	dist := map[NodeID]int{source: 0}
	queue := []NodeID{source}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, nb := range adj.directed[cur] {
			if _, seen := dist[nb]; !seen {
				dist[nb] = dist[cur] + 1
				queue = append(queue, nb)
			}
		}
	}
	return dist
}

// Diameter is the longest shortest path between any two nodes in the same
// component, over the UNDIRECTED graph — all-pairs BFS (one BFS per node;
// this repo's demo scale has no need for Floyd–Warshall-at-scale),
// graph-algorithms.html's own "all-pairs BFS for diameter" row.
// Disconnected pairs are skipped rather than counted as infinite: the
// diameter of a graph with more than one component is conventionally the
// largest per-component diameter, not undefined for the whole graph.
func Diameter(g Graph) int {
	max := 0
	for _, n := range g.Nodes {
		dist, _ := BFS(g, n)
		for _, d := range dist {
			if d > max {
				max = d
			}
		}
	}
	return max
}
