package graphrest

import documentv1 "marginal/document-service/genproto/documentv1"

type graphNodeJSON struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	IsRoot bool   `json:"is_root"`
}

type graphEdgeJSON struct {
	FromPage string `json:"from_page"`
	ToPage   string `json:"to_page"`
}

type linkGraphJSON struct {
	Nodes []graphNodeJSON `json:"nodes"`
	Edges []graphEdgeJSON `json:"edges"`
}

func toLinkGraphJSON(g *documentv1.LinkGraph) linkGraphJSON {
	out := linkGraphJSON{
		Nodes: make([]graphNodeJSON, len(g.GetNodes())),
		Edges: make([]graphEdgeJSON, len(g.GetEdges())),
	}
	for i, n := range g.GetNodes() {
		out.Nodes[i] = graphNodeJSON{ID: n.GetId(), Title: n.GetTitle(), IsRoot: n.GetIsRoot()}
	}
	for i, e := range g.GetEdges() {
		out.Edges[i] = graphEdgeJSON{FromPage: e.GetFromPage(), ToPage: e.GetToPage()}
	}
	return out
}

type bettiNumbersJSON struct {
	B0        int32 `json:"b0"`
	B1        int32 `json:"b1"`
	B1Clique  int32 `json:"b1_clique"`
	B2        int32 `json:"b2"`
	Chi       int32 `json:"chi"`
	Triangles int32 `json:"triangles"`
	Rank2     int32 `json:"rank2"`
}

type graphAnalysisJSON struct {
	ComponentOf      map[string]int32 `json:"component_of"`
	OrphanComponents []int32          `json:"orphan_components"`
	Cycle            []string         `json:"cycle"`
	Diameter         int32            `json:"diameter"`
	Betti            bettiNumbersJSON `json:"betti"`
	// Per-node Brandes centrality, normalised to [0,1]. Always present and
	// empty rather than null, so a client can index it without a guard.
	Betweenness map[string]float64 `json:"betweenness"`
	// Newman's Q against two partitions. Sent together on purpose: either
	// alone is a number with nothing to read it against.
	ModularityByTopic     float64 `json:"modularity_by_topic"`
	ModularityByComponent float64 `json:"modularity_by_component"`
}

func toGraphAnalysisJSON(a *documentv1.GraphAnalysis) graphAnalysisJSON {
	orphans := a.GetOrphanComponents()
	if orphans == nil {
		orphans = []int32{}
	}
	cycle := a.GetCycle()
	if cycle == nil {
		cycle = []string{}
	}
	b := a.GetBetti()
	bc := a.GetBetweenness()
	if bc == nil {
		bc = map[string]float64{}
	}
	return graphAnalysisJSON{
		Betweenness:           bc,
		ModularityByTopic:     a.GetModularityByTopic(),
		ModularityByComponent: a.GetModularityByComponent(),
		ComponentOf:      a.GetComponentOf(),
		OrphanComponents: orphans,
		Cycle:            cycle,
		Diameter:         a.GetDiameter(),
		Betti: bettiNumbersJSON{
			B0: b.GetB0(), B1: b.GetB1(), B1Clique: b.GetB1Clique(), B2: b.GetB2(),
			Chi: b.GetChi(), Triangles: b.GetTriangles(), Rank2: b.GetRank2(),
		},
	}
}

type neighborhoodJSON struct {
	UndirectedDistance map[string]int32 `json:"undirected_distance"`
	ForwardReachable   map[string]int32 `json:"forward_reachable"`
}

func toNeighborhoodJSON(n *documentv1.GraphNeighborhoodResponse) neighborhoodJSON {
	return neighborhoodJSON{
		UndirectedDistance: n.GetUndirectedDistance(),
		ForwardReachable:   n.GetForwardReachable(),
	}
}
