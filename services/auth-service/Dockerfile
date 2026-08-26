# syntax=docker/dockerfile:1
#
# Build context is this directory (services/auth-service/), not the repo
# root — see services/document-service/Dockerfile's comment for why.
# Build with:
#   docker build -t auth-service -f services/auth-service/Dockerfile services/auth-service
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/auth-service ./cmd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/auth-service /auth-service
USER nonroot:nonroot
EXPOSE 8006 9006
ENTRYPOINT ["/auth-service"]
