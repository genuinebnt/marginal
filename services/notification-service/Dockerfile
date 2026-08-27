# syntax=docker/dockerfile:1
#
# Build context is the REPO ROOT, not this directory — cmd/main.go
# imports marginal/envconfig directly across a module boundary (the
# review-fix pass deduplicating cmd/main.go boilerplate —
# docs/porting/PROGRESS.md), resolved purely through go.work's workspace
# mode, the same reason services/document-service/Dockerfile's comment
# gives for that service. Build with (from the repo root):
#   docker build -t notification-service -f services/notification-service/Dockerfile .
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.work ./
COPY services/ services/
WORKDIR /src/services/notification-service
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/notification-service ./cmd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/notification-service /notification-service
USER nonroot:nonroot
EXPOSE 8007
ENTRYPOINT ["/notification-service"]
