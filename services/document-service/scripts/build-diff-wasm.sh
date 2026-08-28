#!/usr/bin/env bash
# Builds cmd/diffwasm's GOOS=js/wasm binary into web/public/, the same
# convention build-wasm.sh/build-graph-wasm.sh already use — wasm_exec.js
# only needs copying once, all three binaries share the same Go runtime
# glue. Regenerate after any textdiff or cmd/diffwasm change — the built
# .wasm is gitignored (build output, not source).
set -euo pipefail
cd "$(dirname "$0")/.."

GOOS=js GOARCH=wasm go build -o ../../web/public/diff.wasm ./cmd/diffwasm
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" ../../web/public/wasm_exec.js

echo "built web/public/diff.wasm ($(du -h ../../web/public/diff.wasm | cut -f1))"
