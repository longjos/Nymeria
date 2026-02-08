# Nymeria

A modern APRS client built with Go and Svelte.

## Features

- Real-time APRS station tracking on an interactive map
- APRS messaging with delivery confirmation
- Multiple transport support: APRS-IS, KISS TCP, Serial TNC
- Multi-user web interface with WebSocket updates
- Single binary deployment with embedded UI

## Quick Start

### Prerequisites

- Go 1.24+
- Node.js 22+ with pnpm
- Make

### Build

```bash
make build
```

### Run

```bash
cp nymeria.example.yaml nymeria.yaml
# Edit nymeria.yaml with your callsign and transport settings
./nymeria
```

Open http://localhost:8080 in your browser.

### Docker

```bash
docker compose up -d
```

### Development

Run the frontend dev server with hot reload and the Go backend:

```bash
make dev
```

## Configuration

See `nymeria.example.yaml` for all configuration options.

## Architecture

Nymeria is built as a single Go binary with an embedded SvelteKit SPA:

- **Backend**: Go with chi router, WebSocket hub, pluggable transport layer
- **Frontend**: SvelteKit with adapter-static, Leaflet maps, TypeScript
- **Build**: `go:embed` bundles the compiled frontend into the binary

## License

MIT
