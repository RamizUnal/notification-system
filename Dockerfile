# syntax=docker/dockerfile:1.7

# ---- build: compiles every command (api, worker, migrate, mockprovider) ----
FROM golang:1.27-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags="-s -w" -o /out/ ./cmd/...

# ---- runtime: one small image, the command picks the process ----
FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 app
COPY --from=build /out/ /usr/local/bin/
USER app
EXPOSE 8080 9090 8090
CMD ["api"]

# ---- test: toolchain, sources and modules for the containerized test run ----
# docker compose --profile test run --rm test
FROM golang:1.27-alpine AS test
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
CMD ["go", "test", "-tags", "integration", "-count=1", "-p", "1", "./..."]
