// Command mdcwasm compiles marginal/mdc to GOOS=js GOARCH=wasm — the
// paste-and-import pipeline, running in the browser.
//
// Client-side because it runs on a paste and on every keystroke of § 11's
// editable buffer. A network round trip is not a paste, and a lab screen that
// recomputes over the wire is a lab screen you watch rather than one you use.
//
// Same JSON-in/JSON-out bridge as every other wasm entrypoint, and in the
// same directory by convention.
//
// Build: GOOS=js GOARCH=wasm go build -o ../../../../web/public/mdc.wasm .
//
//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"marginal/mdc"
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

type compileRequest struct {
	Src string `json:"src"`
}

func compile(this js.Value, args []js.Value) any {
	if len(args) != 1 {
		return result(nil, fmt.Errorf("mdcwasm: compile: expected 1 argument, got %d", len(args)))
	}
	var req compileRequest
	if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
		return result(nil, err)
	}
	data, err := json.Marshal(mdc.Compile(req.Src))
	return result(data, err)
}

func main() {
	js.Global().Set("mdcCompile", js.FuncOf(compile))

	select {}
}
