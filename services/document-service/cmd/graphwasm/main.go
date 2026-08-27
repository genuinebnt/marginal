// Command graphwasm compiles graphalgo's interactive pieces to
// GOOS=js GOARCH=wasm: the force-directed layout and the Voronoi/
// Delaunay territory view, both of which have to run client-side at
// interactive framerate (dragging a node, or recomputing cells as
// positions move every frame) rather than round-tripping to
// GraphService over the network once per frame.
//
// The one-shot algorithms (components, cycles, BFS, diameter, Betti) are
// NOT duplicated here — they're computed once per request by
// GraphService (internal/graph), which already imports the same
// graphalgo package. This binary exists only for the two
// algorithms that genuinely need to live in the browser; everything else
// stays server-side, same package, no second implementation either way.
//
// Same JSON-string-in/JSON-string-out bridge convention as
// services/document-service/cmd/wasm (documentcore's own): every
// exported function takes and returns JSON, the same shape a real HTTP
// call would use, so the view layer never has to know the difference
// between "local wasm call" and "network call."
//
// Build: GOOS=js GOARCH=wasm go build -o ../../../../web/public/graph.wasm .
//
//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"marginal/graphalgo"
)

func jsonArg(args []js.Value, i int) []byte { return []byte(args[i].String()) }

func requireArgs(name string, args []js.Value, n int) error {
	if len(args) != n {
		return fmt.Errorf("graphwasm: %s: expected %d argument(s), got %d", name, n, len(args))
	}
	return nil
}

// result mirrors documentcore's own wasm bridge: exactly one of
// value/error is set, so the JS side checks `.error` before touching
// `.value` — there's no exception to catch across this boundary.
func result(value []byte, err error) js.Value {
	obj := map[string]any{}
	if err != nil {
		obj["error"] = err.Error()
		obj["value"] = nil
	} else {
		obj["error"] = nil
		obj["value"] = string(value)
	}
	return js.ValueOf(obj)
}

// --- seedPositions ----------------------------------------------------

type seedPositionsRequest struct {
	NodeIDs []string `json:"node_ids"`
	Seed    int64    `json:"seed"`
	CenterX float64  `json:"center_x"`
	CenterY float64  `json:"center_y"`
	Spread  float64  `json:"spread"`
}

func seedPositions(this js.Value, args []js.Value) any {
	if err := requireArgs("graphSeedPositions", args, 1); err != nil {
		return result(nil, err)
	}
	var req seedPositionsRequest
	if err := json.Unmarshal(jsonArg(args, 0), &req); err != nil {
		return result(nil, err)
	}
	ids := make([]graphalgo.NodeID, len(req.NodeIDs))
	for i, id := range req.NodeIDs {
		ids[i] = graphalgo.NodeID(id)
	}
	nodes := graphalgo.SeedPositions(ids, req.Seed, req.CenterX, req.CenterY, req.Spread)
	data, err := json.Marshal(nodes)
	return result(data, err)
}

// --- layoutTick ---------------------------------------------------------

type layoutTickRequest struct {
	Nodes     []graphalgo.LayoutNode  `json:"nodes"`
	Edges     []wireEdge              `json:"edges"`
	Params    *graphalgo.LayoutParams `json:"params,omitempty"`
	CenterX   float64                 `json:"center_x"`
	CenterY   float64                 `json:"center_y"`
	Alpha     float64                 `json:"alpha"`
	DraggedID string                  `json:"dragged_id,omitempty"`
}

type wireEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type layoutTickResponse struct {
	Nodes []graphalgo.LayoutNode `json:"nodes"`
	Alpha float64                `json:"alpha"` // already decayed via graphalgo.NextAlpha — the caller stores this for its own next call
}

func layoutTick(this js.Value, args []js.Value) any {
	if err := requireArgs("graphLayoutTick", args, 1); err != nil {
		return result(nil, err)
	}
	var req layoutTickRequest
	if err := json.Unmarshal(jsonArg(args, 0), &req); err != nil {
		return result(nil, err)
	}
	params := graphalgo.DefaultLayoutParams()
	if req.Params != nil {
		params = *req.Params
	}
	edges := make([]graphalgo.Edge, len(req.Edges))
	for i, e := range req.Edges {
		edges[i] = graphalgo.Edge{From: graphalgo.NodeID(e.From), To: graphalgo.NodeID(e.To)}
	}
	nodes := graphalgo.LayoutTick(req.Nodes, edges, params, req.CenterX, req.CenterY, req.Alpha, graphalgo.NodeID(req.DraggedID))
	data, err := json.Marshal(layoutTickResponse{Nodes: nodes, Alpha: graphalgo.NextAlpha(req.Alpha)})
	return result(data, err)
}

// --- territory (Voronoi + Delaunay) -------------------------------------

type territoryRequest struct {
	Sites  []graphalgo.Site `json:"sites"`
	Bounds graphalgo.Rect   `json:"bounds"`
}

type territoryResponse struct {
	Cells    []graphalgo.VoronoiCell  `json:"cells"`
	Delaunay []graphalgo.DelaunayPair `json:"delaunay"`
}

func territory(this js.Value, args []js.Value) any {
	if err := requireArgs("graphTerritory", args, 1); err != nil {
		return result(nil, err)
	}
	var req territoryRequest
	if err := json.Unmarshal(jsonArg(args, 0), &req); err != nil {
		return result(nil, err)
	}
	cells := graphalgo.Voronoi(req.Sites, req.Bounds)
	pairs := graphalgo.Delaunay(cells)
	data, err := json.Marshal(territoryResponse{Cells: cells, Delaunay: pairs})
	return result(data, err)
}

func main() {
	js.Global().Set("graphSeedPositions", js.FuncOf(seedPositions))
	js.Global().Set("graphLayoutTick", js.FuncOf(layoutTick))
	js.Global().Set("graphTerritory", js.FuncOf(territory))

	select {}
}
