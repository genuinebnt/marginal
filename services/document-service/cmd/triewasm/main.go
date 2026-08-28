// Command triewasm compiles internal/trie to GOOS=js GOARCH=wasm for
// editor.html's `[[` page-link autocomplete: RELEASES.md's own
// "[[link]]/command autocomplete via a trie while typing" needs
// interactive, per-keystroke response — the same reasoning
// cmd/graphwasm/cmd/diffwasm's own doc comments give for anything that
// has to run against live client state. document-service is where
// every wasm entrypoint in this repo lives (cmd/wasm, cmd/graphwasm,
// cmd/diffwasm), even though the trie itself has nothing service-
// specific about it.
//
// Stateless per call, same convention as every other wasm bridge here:
// the caller passes the full current title list and the prefix on every
// keystroke, and the trie is rebuilt fresh each time rather than kept
// live across calls — a workspace's title count is small enough
// (this repo's own demo scale) that rebuilding costs nothing an editor
// keystroke's own latency budget would notice, and it avoids this
// module ever holding a stale title list after a page is renamed or
// created without an explicit "invalidate" message of its own.
//
// Build: GOOS=js GOARCH=wasm go build -o ../../../../web/public/trie.wasm .
//
//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"marginal/document-service/internal/trie"
)

func jsonArg(args []js.Value, i int) []byte { return []byte(args[i].String()) }

func requireArgs(name string, args []js.Value, n int) error {
	if len(args) != n {
		return fmt.Errorf("triewasm: %s: expected %d argument(s), got %d", name, n, len(args))
	}
	return nil
}

// result mirrors every other wasm bridge here: exactly one of
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

type prefixSearchRequest struct {
	Titles []string `json:"titles"`
	Prefix string   `json:"prefix"`
}

func prefixSearch(this js.Value, args []js.Value) any {
	if err := requireArgs("triePrefixSearch", args, 1); err != nil {
		return result(nil, err)
	}
	var req prefixSearchRequest
	if err := json.Unmarshal(jsonArg(args, 0), &req); err != nil {
		return result(nil, err)
	}

	var t trie.Trie
	for _, title := range req.Titles {
		t.Insert(title)
	}
	matches := t.PrefixSearch(req.Prefix)
	if matches == nil {
		matches = []string{}
	}
	data, err := json.Marshal(matches)
	return result(data, err)
}

func main() {
	js.Global().Set("triePrefixSearch", js.FuncOf(prefixSearch))

	select {}
}
