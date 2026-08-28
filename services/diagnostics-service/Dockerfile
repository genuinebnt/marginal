# syntax=docker/dockerfile:1
#
# Build context is the REPO ROOT, not this directory — this service
# imports marginal/documentcore, marginal/graphalgo, and document-service's
# own genproto package directly across module boundaries, resolved purely
# through go.work's workspace mode, the same reason api-gateway's and
# document-service's own Dockerfiles build from the repo root. Build with
# (from the repo root):
#   docker build -t diagnostics-service -f services/diagnostics-service/Dockerfile .
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.work ./
COPY services/ services/
WORKDIR /src/services/diagnostics-service
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/diagnostics-service ./cmd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/diagnostics-service /diagnostics-service
USER nonroot:nonroot
EXPOSE 8008 9008
ENTRYPOINT ["/diagnostics-service"]
