# syntax=docker/dockerfile:1
#
# Build context is the REPO ROOT, not web/ — the SPA loads its .wasm modules
# from services/document-service, and those resolve through go.work, which
# requires every module it lists to physically exist on disk. Build with
# (from the repo root):
#   docker build -t marginal-web -f web/Dockerfile .
#
# The .wasm files and wasm_exec.js are gitignored build output (see
# web/.gitignore's `public/*.wasm`), so they must be produced here rather than
# copied from a checkout — a clone has none of them.
#
# The entrypoint list is DISCOVERED, not written down. A missing module does
# not fail the build: it leaves the SPA's try_files fallback serving
# index.html under a .wasm URL, so the browser gets `<!do…` where it wanted
# the wasm magic word. That is how mdc.wasm silently became an HTML page, and
# then how sketch, netsim and bench did too — three at once, on a hand-kept
# list whose own comment said "every cmd/*wasm entrypoint needs a pair below".
# A list that must be remembered will eventually not be.

FROM golang:1.25-alpine AS wasm
WORKDIR /src
COPY go.work go.work.sum ./
COPY services/ services/
WORKDIR /src/services/document-service
# The same builds scripts/build-*-wasm.sh do, inlined: those scripts write into
# ../../web/public, which does not exist in this stage.
#
# cmd/wasm is documentcore's, for historical reasons; every other entrypoint is
# cmd/<name>wasm and ships as <name>.wasm. Both rules are mechanical, so the
# loop applies them rather than a person maintaining a list.
RUN set -eux; \
    built=0; \
    for dir in ./cmd/*wasm; do \
        entry="$(basename "$dir")"; \
        case "$entry" in wasm) name=documentcore ;; *) name="${entry%wasm}" ;; esac; \
        GOOS=js GOARCH=wasm go build -o "/out/${name}.wasm" "$dir"; \
        built=$((built + 1)); \
    done; \
    test "$built" -ge 6; \
    ls -la /out; \
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
