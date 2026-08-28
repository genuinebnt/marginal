// Command diffwasm compiles textdiff's LCS+traceback to GOOS=js GOARCH=wasm
// for docs/ui-mockups/v2/index.html § 15 DIFF: "token granularity switching (word ↔
// character), recomputed live" needs interactive, client-side response
// to a toggle — the same reasoning cmd/graphwasm's own doc comment gives
// for the force layout and Voronoi/Delaunay. document-service is where
// every wasm entrypoint in this repo lives (cmd/wasm, cmd/graphwasm),
// even though textdiff itself has nothing to do with pages — the same
// convention build-wasm.sh/build-graph-wasm.sh already set, kept here
// rather than starting a second wasm-hosting service for one more binary.
//
// Same JSON-string-in/JSON-string-out bridge convention as cmd/wasm and
// cmd/graphwasm.
//
// Build: GOOS=js GOARCH=wasm go build -o ../../../../web/public/diff.wasm .
//
//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"marginal/textdiff"
)

func jsonArg(args []js.Value, i int) []byte { return []byte(args[i].String()) }

func requireArgs(name string, args []js.Value, n int) error {
	if len(args) != n {
		return fmt.Errorf("diffwasm: %s: expected %d argument(s), got %d", name, n, len(args))
	}
	return nil
}

// result mirrors documentcore's/graphwasm's own bridge: exactly one of
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

// --- diff ----------------------------------------------------------------

type diffRequest struct {
	A []string `json:"a"`
	B []string `json:"b"`
}

type diffResponse struct {
	Table [][]int           `json:"table"` // textdiff.LCSTable's own output — diff.html's DP-matrix visualization needs every cell, not just the final script
	Ops   []textdiff.DiffOp `json:"ops"`
	// Path is TracebackWithPath's own visited-cell trail — diff.html's
	// "the outlined path is the traceback that becomes the edit script,"
	// drawn from what Go's own walk actually visited, never re-derived
	// in TypeScript.
	Path []textdiff.Coord `json:"path"`
}

func diff(this js.Value, args []js.Value) any {
	if err := requireArgs("textDiff", args, 1); err != nil {
		return result(nil, err)
	}
	var req diffRequest
	if err := json.Unmarshal(jsonArg(args, 0), &req); err != nil {
		return result(nil, err)
	}
	table := textdiff.LCSTable(req.A, req.B)
	ops, path := textdiff.TracebackWithPath(table, req.A, req.B)
	data, err := json.Marshal(diffResponse{Table: table, Ops: ops, Path: path})
	return result(data, err)
}

func main() {
	js.Global().Set("textDiff", js.FuncOf(diff))

	select {}
}
