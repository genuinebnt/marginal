# syntax=docker/dockerfile:1
#
# Build context is this directory (services/notification-service/), not
# the repo root — see services/document-service/Dockerfile's comment for
# why. Build with:
#   docker build -t notification-service -f services/notification-service/Dockerfile services/notification-service
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/notification-service ./cmd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/notification-service /notification-service
USER nonroot:nonroot
EXPOSE 8007
ENTRYPOINT ["/notification-service"]
