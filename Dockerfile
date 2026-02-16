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

# Stage 2: Build backend (pure Go, no CGO — modernc.org/sqlite is CGO-free)
FROM golang:1.24-alpine AS backend
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/build web/build
ARG VERSION=docker
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o nymeria ./cmd/nymeria

# Stage 3: Runtime image
FROM alpine:3.21
RUN apk add --no-cache ca-certificates wget
WORKDIR /app
COPY --from=backend /app/nymeria .
COPY --from=backend /app/nymeria.example.yaml ./nymeria.yaml

EXPOSE 8080
VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
    CMD wget -qO- http://localhost:8080/api/health || exit 1

ENTRYPOINT ["./nymeria"]
CMD ["--config", "/app/nymeria.yaml", "--listen", ":8080"]
