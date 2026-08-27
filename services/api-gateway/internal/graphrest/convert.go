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

type graphAnalysisJSON struct {
	ComponentOf      map[string]int32 `json:"component_of"`
	OrphanComponents []int32          `json:"orphan_components"`
	Cycle            []string         `json:"cycle"`
	Diameter         int32            `json:"diameter"`
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
	return graphAnalysisJSON{
		ComponentOf:      a.GetComponentOf(),
		OrphanComponents: orphans,
		Cycle:            cycle,
		Diameter:         a.GetDiameter(),
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
