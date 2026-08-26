#!/usr/bin/env bash
# Regenerates genproto/documentv1 from proto/document.proto. Not under
# internal/ — api-gateway (a separate Go module) needs to import the
# generated client stub, and Go's internal-package visibility rule would
# block that across module boundaries.
# Requires protoc, protoc-gen-go, and protoc-gen-go-grpc on PATH
# (`go install google.golang.org/protobuf/cmd/protoc-gen-go@latest` and
# `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest`).
set -euo pipefail
cd "$(dirname "$0")/.."

PROTOBUF_INCLUDE="$(dirname "$(dirname "$(command -v protoc)")")/include"
if [ ! -f "$PROTOBUF_INCLUDE/google/protobuf/timestamp.proto" ]; then
  # Homebrew on Apple Silicon installs protoc's well-known types under the
  # versioned Cellar path, not alongside the protoc binary itself.
  PROTOBUF_INCLUDE="$(find /opt/homebrew/Cellar/protobuf -maxdepth 2 -name include -print -quit 2>/dev/null)"
fi

protoc \
  -I proto \
  -I "$PROTOBUF_INCLUDE" \
  --go_out=. --go_opt=module=marginal/document-service \
  --go-grpc_out=. --go-grpc_opt=module=marginal/document-service \
  proto/document.proto

echo "regenerated genproto/documentv1"
