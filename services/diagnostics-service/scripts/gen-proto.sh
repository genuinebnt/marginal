#!/usr/bin/env bash
# Regenerates genproto/diagnosticsv1 from proto/diagnostics.proto — same
# script shape as document-service's own scripts/gen-proto.sh. Not under
# internal/ for the same reason as that service's genproto: any future
# api-gateway REST shim (a separate Go module) needs to import the
# generated client stub across module boundaries.
# Requires protoc, protoc-gen-go, and protoc-gen-go-grpc on PATH
# (`go install google.golang.org/protobuf/cmd/protoc-gen-go@latest` and
# `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest`).
set -euo pipefail
cd "$(dirname "$0")/.."

PROTOBUF_INCLUDE="$(dirname "$(dirname "$(command -v protoc)")")/include"
if [ ! -f "$PROTOBUF_INCLUDE/google/protobuf/timestamp.proto" ]; then
  PROTOBUF_INCLUDE="$(find /opt/homebrew/Cellar/protobuf -maxdepth 2 -name include -print -quit 2>/dev/null)"
fi

protoc \
  -I proto \
  -I "$PROTOBUF_INCLUDE" \
  --go_out=. --go_opt=module=marginal/diagnostics-service \
  --go-grpc_out=. --go-grpc_opt=module=marginal/diagnostics-service \
  proto/diagnostics.proto

echo "regenerated genproto/diagnosticsv1"
