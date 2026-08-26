# syntax=docker/dockerfile:1
#
# Build context is the REPO ROOT, not this directory — unlike every other
# service's Dockerfile. api-gateway imports document-service's and
# auth-service's genproto packages directly across module boundaries
# (docs/porting/PROGRESS.md's api-gateway entry), resolved purely through
# go.work's workspace mode: every module go.work lists must physically
# exist on disk for that resolution to work at all, so the build context
# needs the whole services/ tree, not just services/api-gateway/. Build
# with (from the repo root):
#   docker build -t api-gateway -f services/api-gateway/Dockerfile .
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.work ./
COPY services/ services/
WORKDIR /src/services/api-gateway
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api-gateway ./cmd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api-gateway /api-gateway
USER nonroot:nonroot
EXPOSE 8000
ENTRYPOINT ["/api-gateway"]
