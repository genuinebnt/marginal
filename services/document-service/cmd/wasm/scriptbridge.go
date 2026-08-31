//go:build js && wasm

package main

import (
	"syscall/js"

	"marginal/document-service/internal/opscript"
)

// replayScriptJS is § 13 TRACE's playground bridge. All of the work is
// in internal/opscript, which is an ordinary package precisely so its
// invertibility check can be tested with `go test` — an algorithm
// behind a js/wasm build tag cannot be run by the test binary at all.
func replayScriptJS(this js.Value, args []js.Value) any {
	if err := requireArgs("documentcoreReplayScript", args, 1); err != nil {
		return result(nil, err)
	}
	data, err := marshal(opscript.Replay(args[0].String()))
	return result(data, err)
}
