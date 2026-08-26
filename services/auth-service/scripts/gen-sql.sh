#!/usr/bin/env bash
# Regenerates internal/authrepo/gen from migrations/ + queries.sql.
set -euo pipefail
cd "$(dirname "$0")/.."
sqlc generate
echo "regenerated internal/authrepo/gen"
