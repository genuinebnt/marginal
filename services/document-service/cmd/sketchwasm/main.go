// Command sketchwasm compiles marginal/sketch to GOOS=js GOARCH=wasm —
// § 12 ANALYTICS's three structures, running in the browser.
//
// Client-side because § 12's event stream is EDITABLE and every panel
// recomputes as you type. That is the point of the screen: a sketch shown
// beside its exact answer is only an argument if you can change the input and
// watch the gap move.
//
// Build: GOOS=js GOARCH=wasm go build -o ../../../../web/public/sketch.wasm .
//
//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"marginal/sketch"
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

type analyzeRequest struct {
	Stream string `json:"stream"`
}

func analyze(this js.Value, args []js.Value) any {
	if len(args) != 1 {
		return result(nil, fmt.Errorf("sketchwasm: analyze: expected 1 argument, got %d", len(args)))
	}
	var req analyzeRequest
	if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
		return result(nil, err)
	}
	events, skipped := sketch.ParseStream(req.Stream)
	report := sketch.Analyze(events)
	report.Skipped = skipped
	data, err := json.Marshal(report)
	return result(data, err)
}

func main() {
	js.Global().Set("sketchAnalyze", js.FuncOf(analyze))

	select {}
}
