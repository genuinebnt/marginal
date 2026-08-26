# syntax=docker/dockerfile:1
#
# Build context is the REPO ROOT, not this directory — unlike most other
# services' Dockerfiles (see services/document-service/Dockerfile's
# comment for the general rule). internal/session and internal/pageop
# import marginal/documentcore directly across a module boundary (the
# block-op reconciliation pass — docs/porting/PROGRESS.md), resolved
# purely through go.work's workspace mode, the same reason
# services/api-gateway/Dockerfile builds from the repo root. Build with
# (from the repo root):
#   docker build -t collaboration-service -f services/collaboration-service/Dockerfile .
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.work ./
COPY services/ services/
WORKDIR /src/services/collaboration-service
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/collaboration-service ./cmd

# Runs as root (distroless's default user), not :nonroot like the other
# services — this service writes a local WAL file per open page
# (internal/wal) under COLLAB_WAL_DIR, typically a mounted volume that a
# non-root UID has no guaranteed write access to without extra chown
# machinery this Dockerfile doesn't add. Revisit if this ever needs to run
# with a locked-down Pod Security Standard that forbids root.
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/collaboration-service /collaboration-service
EXPOSE 8002
ENTRYPOINT ["/collaboration-service"]
