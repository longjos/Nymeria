# Stage 1: Build frontend
FROM node:22-alpine AS frontend
WORKDIR /app/web
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

# Stage 2: Build backend
FROM golang:1.24-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/build web/build
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o nymeria ./cmd/nymeria

# Stage 3: Final image
FROM alpine:3.20
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=backend /app/nymeria .
COPY --from=backend /app/nymeria.example.yaml ./nymeria.yaml
EXPOSE 8080
ENTRYPOINT ["./nymeria"]
