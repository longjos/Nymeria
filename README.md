# Nymeria

A modern, full-featured APRS client built with Go and SvelteKit. Nymeria runs as a single binary with an embedded web interface — deploy it on a Raspberry Pi, a server, or your desktop and access it from any browser.

## Features

**Core APRS**
- Real-time station tracking on an interactive Leaflet map
- Full APRS packet parsing: position, messaging, weather, telemetry, objects/items
- APRS messaging with delivery confirmation and automatic retry
- Position beaconing (fixed interval and manual trigger)
- Object and item creation, retransmission, and kill
- Bulletin and announcement support

**Transports**
- **APRS-IS** — Internet gateway with server-side filtering
- **KISS TCP** — Direwolf or other software TNCs
- **Serial KISS** — Hardware TNCs via serial port
- Dual transport with automatic cross-transport packet deduplication

**Multi-User**
- Session management with 4-tier roles (Admin, Operator, Plotter, Observer)
- Admin-approved invite flow — zero-friction field deployment, no PINs or passwords
- Message conversation claiming per operator
- Activity logging with CSV export

**Dashboards**
- Weather station dashboard with charts and map overlay
- Telemetry display with PARM/UNIT/EQNS metadata parsing and graphing
- Direction finding overlay with bearing lines and triangulation
- Protocol packet inspector

**Emergency Communications**
- Net control dashboard with roster, missions, timeline, roll call, and NCS transfer
- NCS command palette for high-tempo keyboard-driven operations
- Agency summary dashboard for EOC/ICP display — real-time multi-net overview
- Annotations (points, lines, areas) with net-scoping, GPX/KML import, and APRS object bridge
- Checkpoint and event progress tracking with route progress bar
- ICS-309 communications log generation
- First-run setup wizard with guided configuration

**Map & Overlays**
- Station age filtering, track/DR cone toggles, weather and DF overlays
- Offline map tile caching
- Station categories with priority-based sorting
- APRS-IS filter builder with visual rule editor

**Deployment**
- Single binary with embedded frontend via `go:embed`
- SQLite storage — no external database needed
- Docker Compose setup with Direwolf for turnkey RF stations
- Cross-platform: Linux, Windows, macOS (amd64 and arm64)
- [Pre-built binaries](https://github.com/longjos/Nymeria/releases) available

## Quick Start

### Install Options

| Option | Artifact / command | When to pick it |
|--------|--------------------|-----------------|
| **Windows desktop** | `Nymeria-desktop-windows-amd64.exe` from [Releases](https://github.com/longjos/Nymeria/releases) | Native tray app on a Windows laptop; requires WebView2 (preinstalled on Windows 11 and most Windows 10) |
| **Headless binaries** | `nymeria-linux-amd64`, `nymeria-linux-arm64`, `nymeria.exe` | Servers, Raspberry Pi, or any machine where you open the UI in a browser |
| **Docker** | `docker compose up` | Turnkey RF station bundled with Direwolf |

### Prerequisites

- Go 1.24+
- Node.js 22+ with pnpm
- Make

### Build and Run

```bash
git clone https://github.com/longjos/Nymeria.git
cd Nymeria
make build
cp nymeria.example.yaml nymeria.yaml
# Edit nymeria.yaml with your callsign and transport settings
./nymeria --listen :8080
```

Open http://localhost:8080 in your browser. On first launch, a setup wizard walks you through callsign and APRS-IS configuration.

### Docker Compose (with Direwolf)

For a turnkey RF station with a software TNC:

```bash
cp docker/direwolf.conf.example docker/direwolf.conf
cp docker/nymeria.yaml.example docker/nymeria.yaml
# Edit both files with your callsign and audio device
docker compose up -d
```

This starts Direwolf (software TNC with soundcard passthrough) and Nymeria connected via KISS TCP. Works on amd64 and arm64.

## Configuration

Copy `nymeria.example.yaml` and edit to taste:

```yaml
server:
  listen: ":8080"

station:
  callsign: "N0CALL"        # Your amateur radio callsign
  ssid: 0
  lat: 35.0                 # Station latitude
  lon: -106.0               # Station longitude
  stale_timeout: 80m         # Remove stations after this

transports:
  # APRS-IS internet gateway
  - type: aprsis
    host: rotate.aprs2.net
    port: 14580
    filter: "r/35.0/-106.0/100"

  # Direwolf KISS TCP
  # - type: kisstcp
  #   host: localhost
  #   port: 8001

  # Hardware TNC via serial
  # - type: serial
  #   device: /dev/ttyUSB0
  #   baud: 9600

store:
  path: "./nymeria.db"

beacon:
  enabled: false
  interval: 10m

session:
  pin: ""                    # Set a PIN to require authentication

logging:
  level: "info"
```

### Environment Variable Overrides

| Variable | Description |
|----------|-------------|
| `NYMERIA_LISTEN` | Server listen address |
| `NYMERIA_CALLSIGN` | Station callsign |
| `NYMERIA_DB_PATH` | SQLite database path |
| `NYMERIA_LOG_LEVEL` | Log level (debug, info, warn, error) |

### CLI Flags

```
./nymeria [flags]

  --config PATH    Config file path (default: nymeria.yaml)
  --listen ADDR    Override listen address (e.g., :9090)
  --version        Print version and exit
```

## Development

### Build Targets

```bash
make build      # Build frontend + backend
make frontend   # Build SvelteKit frontend only
make backend    # Build Go binary only
make test       # Run all Go tests
make lint       # Run go vet
make dev        # Frontend dev server + Go backend with hot reload
make windows    # Cross-compile for Windows (amd64)
make desktop-windows   # Cross-compile the Windows desktop app (Wails v3)
make docker     # Build Docker image
make clean      # Remove build artifacts
```

### Project Structure

```
cmd/nymeria/           Entry point, CLI flags, wiring
internal/
  aprs/                APRS protocol: frame, parser, position, symbol, AX.25, KISS
  transport/           Pluggable transport interface, manager, deduplication
    aprsis/            APRS-IS transport
    kisstcp/           KISS TCP transport
    serial/            Serial KISS transport
  station/             Station tracking (in-memory + persistence)
  message/             APRS messaging engine with ack/retry
  beacon/              Position beaconing
  object/              APRS object/item management
  session/             Multi-user sessions and roles
  store/               SQLite persistence layer
  config/              YAML config loading and validation
  server/              Chi HTTP router, REST API, middleware
    ws/                WebSocket hub for real-time broadcasts
  activity/            Activity logging and CSV export
  annotation/          Annotations with net-scoping, GPX/KML import, APRS bridge
  netcontrol/          Net control management (nets, roster, missions, timeline)
  checkpoint/          Checkpoint and event progress tracking
  ics309/              ICS-309 log generation
  tilecache/           Offline map tile caching
web/                   SvelteKit frontend (adapter-static)
  embed.go             go:embed bridge for the compiled frontend
docker/                Docker Compose configs for Direwolf integration
wiki/                  Project documentation (git submodule)
```

### Architecture

Nymeria is a single Go binary with an embedded SvelteKit SPA:

- **Backend**: Go with [chi](https://github.com/go-chi/chi) router, [nhooyr.io/websocket](https://github.com/nhooyr/websocket) for real-time updates, [modernc.org/sqlite](https://modernc.org/sqlite) for storage (pure Go, no CGO required at runtime)
- **Frontend**: SvelteKit 5 with adapter-static, [Leaflet](https://leafletjs.com/) maps, TypeScript
- **Build**: The frontend compiles to static files, then `go:embed` bundles them into the Go binary
- **Transports**: Pluggable interface — APRS-IS, KISS TCP, and Serial all implement the same `Transport` interface, managed by a central `Manager` with automatic deduplication

### Testing

All backend code follows TDD. Tests live next to their source files. The test suite includes race detection:

```bash
make test                    # Standard test run
go test -race ./...          # With race detector
go test -cover ./...         # With coverage
```

## License

MIT
