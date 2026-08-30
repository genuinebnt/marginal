// Command netsimwasm compiles marginal/netsim to GOOS=js GOARCH=wasm —
// § 14 NETCODE's engine, running in the browser.
//
// Client-side because § 14's whole input is a set of controls: an RTT
// slider, a loss percentage, a transform toggle, and an editable
// script of who typed what when. Every one of them re-runs the whole
// simulation, and a screen that asks a server before it can redraw a
// slider is not a debugger.
//
// Build: GOOS=js GOARCH=wasm go build -o ../../../../web/public/netsim.wasm .
//
//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"marginal/netsim"
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
	Script    string       `json:"script"`
	Wire      netsim.Wire  `json:"wire"`
	Transform bool         `json:"transform"`
	Initial   string       `json:"initial"`
}

type runResponse struct {
	netsim.Report
	// Skipped lines of the script. Reported rather than refused, the
	// same rule § 12's stream follows.
	Skipped int `json:"skipped"`
	Edits   int `json:"edits"`
}

func run(this js.Value, args []js.Value) any {
	if len(args) != 1 {
		return result(nil, fmt.Errorf("netsimwasm: run: expected 1 argument, got %d", len(args)))
	}
	var req runRequest
	if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
		return result(nil, err)
	}
	edits, skipped := netsim.ParseScenario(req.Script)
	report := netsim.Run(netsim.Scenario{
		Wire: req.Wire, Transform: req.Transform,
		Initial: req.Initial, Edits: edits,
	})
	data, err := json.Marshal(runResponse{Report: report, Skipped: skipped, Edits: len(edits)})
	return result(data, err)
}

func main() {
	js.Global().Set("netsimRun", js.FuncOf(run))

	select {}
}
