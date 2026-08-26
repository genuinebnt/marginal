# syntax=docker/dockerfile:1
#
# Build context is the REPO ROOT, not this directory — internal/blockproj
# imports marginal/documentcore directly across a module boundary (the
# collab.ops_flushed materialisation pass — docs/porting/PROGRESS.md),
# resolved purely through go.work's workspace mode, the same reason
# services/api-gateway/Dockerfile and services/collaboration-service/Dockerfile
# build from the repo root. Build with (from the repo root):
#   docker build -t document-service -f services/document-service/Dockerfile .
#
# The wasm build (documentcore.wasm, scripts/build-wasm.sh) is a frontend
# asset bundled into web/'s own build — it does not belong in this
# service's own container, which only runs the gRPC/HTTP binary.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.work ./
COPY services/ services/
WORKDIR /src/services/document-service
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/document-service ./cmd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/document-service /document-service
USER nonroot:nonroot
EXPOSE 8001 9001
ENTRYPOINT ["/document-service"]
