// Command benchwasm compiles marginal/bench to GOOS=js
// GOARCH=wasm — § 16 PERF's benchmark, run in the browser.
//
// Client-side because the screen says "measured on your
// machine" and means it: the workloads are real Go paths
// from this repo, compiled to wasm and timed where the
// person reading the numbers is sitting. A server-side
// benchmark would measure the server, which is a different
// and much less interesting claim.
//
// Build: GOOS=js GOARCH=wasm go build -o ../../../../web/public/bench.wasm .
//
//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"marginal/bench"
)

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

type runRequest struct {
	Workload string `json:"workload"`
	Samples  int    `json:"samples"`
}

type listResponse struct {
	Workloads []workloadJSON `json:"workloads"`
}

type workloadJSON struct {
	Name       string `json:"name"`
	Note       string `json:"note"`
	MaxSamples int    `json:"max_samples"`
}

func run(this js.Value, args []js.Value) any {
	if len(args) != 1 {
		return result(nil, fmt.Errorf("benchwasm: run: expected 1 argument, got %d", len(args)))
	}
	var req runRequest
	if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
		return result(nil, err)
	}
	data, err := json.Marshal(bench.Run(bench.ByName(req.Workload), req.Samples))
	return result(data, err)
}

// list lets the screen build its own workload picker from the
// module rather than repeating the names in TypeScript, where
// they would drift the first time one is renamed.
func list(this js.Value, args []js.Value) any {
	out := listResponse{}
	for _, w := range bench.Workloads() {
		out.Workloads = append(out.Workloads, workloadJSON{
			Name: w.Name, Note: w.Note, MaxSamples: w.MaxSamples,
		})
	}
	data, err := json.Marshal(out)
	return result(data, err)
}

func main() {
	js.Global().Set("benchRun", js.FuncOf(run))
	js.Global().Set("benchList", js.FuncOf(list))

	select {}
}
