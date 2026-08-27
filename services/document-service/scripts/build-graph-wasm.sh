#!/usr/bin/env bash
# Builds cmd/graphwasm's GOOS=js/wasm binary into web/public/, where Vite
# serves it as a static asset — the same convention build-wasm.sh already
# uses for documentcore (wasm_exec.js only needs copying once; both
# binaries share the same Go runtime glue). Regenerate after any
# graphalgo or cmd/graphwasm change — the built .wasm is gitignored
# (build output, not source).
set -euo pipefail
cd "$(dirname "$0")/.."

GOOS=js GOARCH=wasm go build -o ../../web/public/graph.wasm ./cmd/graphwasm
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" ../../web/public/wasm_exec.js

echo "built web/public/graph.wasm ($(du -h ../../web/public/graph.wasm | cut -f1))"
