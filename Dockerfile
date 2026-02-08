# Multi-stage build for Nymeria APRS client
# Supports amd64 and arm64 (Raspberry Pi 4)

# Stage 1: Build frontend
FROM node:22-alpine AS frontend
WORKDIR /app/web
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

# Stage 2: Build backend (CGO required for modernc.org/sqlite)
FROM golang:1.24-bookworm AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/build web/build
ARG VERSION=docker
RUN CGO_ENABLED=1 go build -ldflags "-s -w -X main.version=${VERSION}" -o nymeria ./cmd/nymeria

# Stage 3: Runtime image
FROM debian:bookworm-slim
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates wget && \
    rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=backend /app/nymeria .
COPY --from=backend /app/nymeria.example.yaml ./nymeria.yaml

EXPOSE 8080
VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
    CMD wget -qO- http://localhost:8080/api/health || exit 1

ENTRYPOINT ["./nymeria"]
CMD ["--config", "/app/nymeria.yaml", "--listen", ":8080"]
