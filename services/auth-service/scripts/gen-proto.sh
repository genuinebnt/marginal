#!/usr/bin/env bash
# Regenerates internal/genproto/authv1 from proto/auth.proto.
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
  proto/auth.proto

echo "regenerated internal/genproto/authv1"
