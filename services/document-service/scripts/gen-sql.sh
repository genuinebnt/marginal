#!/usr/bin/env bash
# Regenerates internal/pagerepo/gen from migrations/ + queries.sql.
# Requires sqlc on PATH (`go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`).
set -euo pipefail
cd "$(dirname "$0")/.."
sqlc generate
echo "regenerated internal/pagerepo/gen"
