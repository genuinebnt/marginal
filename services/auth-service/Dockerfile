# syntax=docker/dockerfile:1
#
# Build context is the REPO ROOT, not this directory — internal/outbox
# imports marginal/envconfig and marginal/outboxpoll directly across
# module boundaries (the review-fix pass deduplicating cmd/main.go
# boilerplate and the outbox Poller mechanism — docs/porting/PROGRESS.md),
# resolved purely through go.work's workspace mode, the same reason
# services/document-service/Dockerfile's comment gives for that service.
# Build with (from the repo root):
#   docker build -t auth-service -f services/auth-service/Dockerfile .
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.work ./
COPY services/ services/
WORKDIR /src/services/auth-service
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/auth-service ./cmd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/auth-service /auth-service
USER nonroot:nonroot
EXPOSE 8006 9006
ENTRYPOINT ["/auth-service"]
