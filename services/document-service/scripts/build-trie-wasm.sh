#!/usr/bin/env bash
# Builds cmd/triewasm's GOOS=js/wasm binary into web/public/, the same
# convention build-wasm.sh/build-graph-wasm.sh/build-diff-wasm.sh already
# use — wasm_exec.js only needs copying once. Regenerate after any trie
# or cmd/triewasm change — the built .wasm is gitignored (build output,
# not source).
set -euo pipefail
cd "$(dirname "$0")/.."

GOOS=js GOARCH=wasm go build -o ../../web/public/trie.wasm ./cmd/triewasm
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" ../../web/public/wasm_exec.js

echo "built web/public/trie.wasm ($(du -h ../../web/public/trie.wasm | cut -f1))"
