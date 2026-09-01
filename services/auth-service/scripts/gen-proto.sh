#!/usr/bin/env bash
# Regenerates genproto/authv1 from every proto/*.proto. Not under internal/ — api-gateway (a separate Go module) needs to import the generated client stub, and Go's internal-package visibility rule would block that across module boundaries.
set -euo pipefail
cd "$(dirname "$0")/.."

PROTOBUF_INCLUDE="$(dirname "$(dirname "$(command -v protoc)")")/include"
if [ ! -f "$PROTOBUF_INCLUDE/google/protobuf/timestamp.proto" ]; then
  PROTOBUF_INCLUDE="$(find /opt/homebrew/Cellar/protobuf -maxdepth 2 -name include -print -quit 2>/dev/null)"
fi

protoc \
  -I proto \
  -I "$PROTOBUF_INCLUDE" \
  --go_out=. --go_opt=module=marginal/auth-service \
  --go-grpc_out=. --go-grpc_opt=module=marginal/auth-service \
  proto/*.proto

echo "regenerated genproto/authv1"
