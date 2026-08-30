package graphrest

import documentv1 "marginal/document-service/genproto/documentv1"

type graphNodeJSON struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	IsRoot bool   `json:"is_root"`
	// Empty when untopiced — a real state, drawn in its own hue rather than
	// one of the five. Carried here so a client never has to join ListPages,
	// which returns one parent's children and silently covers only the roots.
	TopicName     string   `json:"topic_name"`
	TopicColorKey string   `json:"topic_color_key"`
	Tags          []string `json:"tags"`
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
		out.Nodes[i] = graphNodeJSON{
			ID: n.GetId(), Title: n.GetTitle(), IsRoot: n.GetIsRoot(),
			TopicName: n.GetTopicName(), TopicColorKey: n.GetTopicColorKey(),
			Tags: emptyTags(n.GetTags()),
		}
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
	// Strong connectivity over the DIRECTED graph — "can I get there AND
	// back", where component_of asks only "can I get there at all".
	StronglyConnected map[string]int32 `json:"strongly_connected"`
	SccSizes          []int32          `json:"scc_sizes"`
	// A reading order where nothing precedes what it links to. PARTIAL when
	// is_dag is false; `unplaced` then holds what could not be ordered.
	TopologicalOrder []string   `json:"topological_order"`
	IsDAG            bool       `json:"is_dag"`
	Unplaced         []string   `json:"unplaced"`
	Layers           [][]string `json:"layers"`
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
	// Every slice below ships empty rather than null: a client indexing or
	// iterating these should never need a guard for "the server had nothing".
	scc := a.GetStronglyConnected()
	if scc == nil {
		scc = map[string]int32{}
	}
	sccSizes := a.GetSccSizes()
	if sccSizes == nil {
		sccSizes = []int32{}
	}
	topo := a.GetTopologicalOrder()
	if topo == nil {
		topo = []string{}
	}
	unplaced := a.GetUnplaced()
	if unplaced == nil {
		unplaced = []string{}
	}
	layers := make([][]string, 0, len(a.GetLayers()))
	for _, l := range a.GetLayers() {
		ids := l.GetPageIds()
		if ids == nil {
			ids = []string{}
		}
		layers = append(layers, ids)
	}
	return graphAnalysisJSON{
		StronglyConnected:     scc,
		SccSizes:              sccSizes,
		TopologicalOrder:      topo,
		IsDAG:                 a.GetIsDag(),
		Unplaced:              unplaced,
		Layers:                layers,
		Betweenness:           bc,
		ModularityByTopic:     a.GetModularityByTopic(),
		ModularityByComponent: a.GetModularityByComponent(),
		ComponentOf:           a.GetComponentOf(),
		OrphanComponents:      orphans,
		Cycle:                 cycle,
		Diameter:              a.GetDiameter(),
		Betti: bettiNumbersJSON{
			B0: b.GetB0(), B1: b.GetB1(), B1Clique: b.GetB1Clique(), B2: b.GetB2(),
			Chi: b.GetChi(), Triangles: b.GetTriangles(), Rank2: b.GetRank2(),
		},
	}
}

type graphNeighbourJSON struct {
	PageID string `json:"page_id"`
	Title  string `json:"title"`
	Hops   int32  `json:"hops"`
}

type neighborhoodJSON struct {
	UndirectedDistance map[string]int32 `json:"undirected_distance"`
	ForwardReachable   map[string]int32 `json:"forward_reachable"`
	// The ranked ring, nearest first. Near BY LINKS — a different question
	// from near by meaning, and the gap between the two is the finding.
	Nearest []graphNeighbourJSON `json:"nearest"`
	// ring_sizes[d] is how many pages sit exactly d hops out, from d = 0.
	RingSizes []int32 `json:"ring_sizes"`
	// "Read these, in this order" — everything that reaches this page by
	// following links forward, layered, ending at the page itself.
	ReadingPath []pathStepJSON `json:"reading_path"`
	// source → target, one entry per hop, in order, when ?to= named a
	// target. Undirected, matching undirected_distance: "how are these
	// two connected" does not care which way a link points.
	ShortestPath []pathStepJSON `json:"shortest_path"`
	// Empty shortest_path means two different things — nobody asked, or
	// there is no route — and this is what tells them apart.
	PathExists bool `json:"path_exists"`
}

type pathStepJSON struct {
	PageID      string `json:"page_id"`
	Title       string `json:"title"`
	Depth       int32  `json:"depth"`
	Destination bool   `json:"destination"`
}

func toNeighborhoodJSON(n *documentv1.GraphNeighborhoodResponse) neighborhoodJSON {
	nearest := make([]graphNeighbourJSON, 0, len(n.GetNearest()))
	for _, nb := range n.GetNearest() {
		nearest = append(nearest, graphNeighbourJSON{
			PageID: nb.GetPageId(), Title: nb.GetTitle(), Hops: nb.GetHops(),
		})
	}
	rings := n.GetRingSizes()
	if rings == nil {
		rings = []int32{}
	}
	path := make([]pathStepJSON, 0, len(n.GetReadingPath()))
	for _, s := range n.GetReadingPath() {
		path = append(path, pathStepJSON{
			PageID: s.GetPageId(), Title: s.GetTitle(),
			Depth: s.GetDepth(), Destination: s.GetDestination(),
		})
	}
	shortest := make([]pathStepJSON, 0, len(n.GetShortestPath()))
	for _, s := range n.GetShortestPath() {
		shortest = append(shortest, pathStepJSON{
			PageID: s.GetPageId(), Title: s.GetTitle(),
			Depth: s.GetDepth(), Destination: s.GetDestination(),
		})
	}
	return neighborhoodJSON{
		ReadingPath:        path,
		UndirectedDistance: n.GetUndirectedDistance(),
		ForwardReachable:   n.GetForwardReachable(),
		Nearest:            nearest,
		RingSizes:          rings,
		ShortestPath:       shortest,
		PathExists:         n.GetPathExists(),
	}
}

// emptyTags ships `[]` rather than `null` — a client iterating tags should
// not need a guard for "this page has none".
func emptyTags(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
