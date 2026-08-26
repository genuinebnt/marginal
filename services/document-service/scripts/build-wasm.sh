#!/usr/bin/env bash
# Builds documentcore's GOOS=js/wasm binary and copies the matching
# wasm_exec.js glue into web/public/, where Vite serves it as a static
# asset. Regenerate after any documentcore or cmd/wasm change — the built
# .wasm and wasm_exec.js are gitignored (build output, not source).
set -euo pipefail
cd "$(dirname "$0")/.."

GOOS=js GOARCH=wasm go build -o ../../web/public/documentcore.wasm ./cmd/wasm
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" ../../web/public/wasm_exec.js

echo "built web/public/documentcore.wasm ($(du -h ../../web/public/documentcore.wasm | cut -f1))"
