// Command syntaxwasm compiles marginal/syntax to GOOS=js GOARCH=wasm — the
// code-block highlighter, running in the browser because it has to run on
// every keystroke inside an open code block and a network round trip per
// keystroke is not a highlighter.
//
// Same JSON-string-in/JSON-string-out bridge convention as every other wasm
// entrypoint here, and in the same directory by convention even though
// marginal/syntax has nothing to do with pages (cmd/diffwasm and
// cmd/triewasm are here for the same reason).
//
// Build: GOOS=js GOARCH=wasm go build -o ../../../../web/public/syntax.wasm .
//
//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"marginal/syntax"
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

type highlightRequest struct {
	Lang string `json:"lang"`
	Src  string `json:"src"`
}

type highlightResponse struct {
	Tokens []syntax.Token `json:"tokens"`
}

func highlight(this js.Value, args []js.Value) any {
	if len(args) != 1 {
		return result(nil, fmt.Errorf("syntaxwasm: highlight: expected 1 argument, got %d", len(args)))
	}
	var req highlightRequest
	if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
		return result(nil, err)
	}
	data, err := json.Marshal(highlightResponse{Tokens: syntax.Highlight(req.Lang, req.Src)})
	return result(data, err)
}

func languages(this js.Value, args []js.Value) any {
	data, err := json.Marshal(syntax.Languages())
	return result(data, err)
}

func main() {
	js.Global().Set("syntaxHighlight", js.FuncOf(highlight))
	js.Global().Set("syntaxLanguages", js.FuncOf(languages))

	select {}
}
