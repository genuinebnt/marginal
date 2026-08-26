// Command wasm compiles documentcore to GOOS=js GOARCH=wasm — the editor
// core's business logic, callable from the browser. Per the user's
// direction: business logic lives in Go (here, compiled to wasm); the
// TypeScript side (web/src/document-core's loader + a thin History
// bookkeeping class) is a view/binding layer only, never a second
// implementation of the logic itself.
//
// This is also the same seam ADR-004 always specified for the editor core
// (Rust -> wasm32-unknown-unknown): swapping the Go implementation for the
// Rust one later means recompiling to that target instead of js/wasm, with
// the JS-side loader needing minimal changes — the wasm boundary was never
// removed, only its source language, for now.
//
// Build: GOOS=js GOARCH=wasm go build -o ../../../../web/public/documentcore.wasm .
//
//go:build js && wasm

package main

import (
	"syscall/js"

	"marginal/document-service/internal/documentcore"
)

// jsonArg reads args[i] as a JSON string. Every exported function takes and
// returns JSON strings — a stringly-typed boundary, not a rich js.Value
// marshaling scheme, because it's the same shape a real HTTP/gRPC call to
// document-service would use. The views never need to know the difference
// between "local wasm call" and "network call to the real service."
func jsonArg(args []js.Value, i int) []byte {
	return []byte(args[i].String())
}

// result builds the {value, error} envelope every exported function
// returns: exactly one of the two is set. JS callers check `.error` before
// touching `.value` — there's no exception to catch, since js.Value can't
// carry a Go error across the boundary directly.
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

func newPage(this js.Value, args []js.Value) any {
	var req struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := unmarshal(jsonArg(args, 0), &req); err != nil {
		return result(nil, err)
	}
	id, err := parsePageID(req.ID)
	if err != nil {
		return result(nil, err)
	}
	page := documentcore.NewPage(id, req.Title)
	data, err := marshal(page)
	return result(data, err)
}

func applyOp(this js.Value, args []js.Value) any {
	var page documentcore.Page
	if err := unmarshal(jsonArg(args, 0), &page); err != nil {
		return result(nil, err)
	}
	op, err := documentcore.UnmarshalOp(jsonArg(args, 1))
	if err != nil {
		return result(nil, err)
	}
	if err := page.Apply(op); err != nil {
		return result(nil, err)
	}
	data, err := marshal(page)
	return result(data, err)
}

func invertOp(this js.Value, args []js.Value) any {
	op, err := documentcore.UnmarshalOp(jsonArg(args, 0))
	if err != nil {
		return result(nil, err)
	}
	data, err := documentcore.MarshalOp(op.Invert())
	return result(data, err)
}

func main() {
	js.Global().Set("documentcoreNewPage", js.FuncOf(newPage))
	js.Global().Set("documentcoreApplyOp", js.FuncOf(applyOp))
	js.Global().Set("documentcoreInvertOp", js.FuncOf(invertOp))

	// Registered js.Func callbacks keep firing only while main is still
	// running — block forever rather than returning.
	select {}
}
