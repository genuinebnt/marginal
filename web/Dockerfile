# syntax=docker/dockerfile:1
#
# Build context is the REPO ROOT, not web/ — the SPA loads five .wasm modules
# compiled from services/document-service, and those resolve through go.work,
# which requires every module it lists to physically exist on disk. Build with
# (from the repo root):
#   docker build -t marginal-web -f web/Dockerfile .
#
# The .wasm files and wasm_exec.js are gitignored build output (see
# web/.gitignore's `public/*.wasm`), so they must be produced here rather than
# copied from a checkout — a clone has none of them. Every cmd/*wasm entrypoint
# needs a pair in the loop below; one that is missing does not fail the build,
# it just leaves the SPA's try_files fallback serving index.html under a .wasm
# URL, which is how mdc.wasm silently became an HTML page.

FROM golang:1.25-alpine AS wasm
WORKDIR /src
COPY go.work go.work.sum ./
COPY services/ services/
WORKDIR /src/services/document-service
# Same five builds as scripts/build-*-wasm.sh, inlined: the scripts write into
# ../../web/public, which does not exist in this stage.
RUN set -eux; \
    for pair in wasm:documentcore graphwasm:graph diffwasm:diff triewasm:trie syntaxwasm:syntax mdcwasm:mdc; do \
        GOOS=js GOARCH=wasm go build -o "/out/${pair#*:}.wasm" "./cmd/${pair%%:*}"; \
    done; \
    cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" /out/wasm_exec.js

FROM node:22-alpine AS build
WORKDIR /app
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
# Vite serves public/ at the site root, which is where the loaders expect them.
COPY --from=wasm /out/ ./public/

# Where the browser reaches each backend. These are baked into the bundle at
# build time (import.meta.env), so they are build args, not runtime env — the
# image is specific to the origin it was built for. The prefixes must match the
# handle_path blocks in the gateway's Caddyfile; the SPA's own client routes
# collide with the raw API paths (/discover/:id, /notifications), which is why
# the APIs are namespaced at all.
ARG VITE_GATEWAY_URL=https://marginal.genuinebasil.dev/api
ARG VITE_COLLAB_URL=https://marginal.genuinebasil.dev/ws
ARG VITE_NOTIFICATIONS_URL=https://marginal.genuinebasil.dev/notify
RUN printf 'VITE_GATEWAY_URL=%s\nVITE_COLLAB_URL=%s\nVITE_NOTIFICATIONS_URL=%s\n' \
        "$VITE_GATEWAY_URL" "$VITE_COLLAB_URL" "$VITE_NOTIFICATIONS_URL" \
        > .env.production \
    && npm run build

FROM caddy:2-alpine
COPY web/Caddyfile.static /etc/caddy/Caddyfile
COPY --from=build /app/dist /srv
EXPOSE 80
