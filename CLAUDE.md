# Nymeria

## Development Workflow

**This is the law. Every session follows this order.**

### 1. Backend: Test-Driven Development (TDD)
- **Tests first, always.** Write tests before implementation.
- APRS is a well-documented protocol (APRS101.PDF). The spec defines correctness — use it to write tests against known-good packet examples.
- Test files live next to their source: `parser_test.go` beside `parser.go`.
- Run `make test` after every implementation change. All tests must pass before moving on.
- Table-driven tests preferred for protocol parsing (many inputs, one function).

### 2. Backend: Build APIs & Integrations
- Implement against the interfaces already defined in the stubs.
- Replace stubs in-place — don't create new files unless adding a new package.
- Wire real implementations through `cmd/nymeria/main.go`.
- Verify with `make build && make test`.

### 3. Frontend: World-Class UX
- This is not a prototype UI. Apply real design principles:
  - **Visual hierarchy** — the most important information is the most prominent
  - **Information density** — show what matters, hide what doesn't, progressive disclosure
  - **Consistency** — spacing, color, typography follow a system, not ad hoc choices
  - **Responsiveness** — works on desktop, tablet, and mobile
  - **Accessibility** — keyboard navigation, contrast ratios, semantic HTML
  - **Feedback** — every action has a visible response (loading states, confirmations, errors)
- Design tokens (colors, spacing, type scale) defined in CSS variables in `app.css`
- Components in `web/src/lib/components/` — reusable, composable, typed props
- No placeholder "coming soon" pages. If a feature isn't built, the route doesn't exist.

### 4. Documentation: Update Wiki After Each Build
- After completing a feature or issue, update `wiki/Home.md`:
  - Check off the completed item in the Development Roadmap
  - Add a "Current Status" or "Changelog" section documenting what works now
- Commit wiki changes with the feature: `cd wiki && git add -A && git commit -m "..." && cd .. && git add wiki && git commit -m "Update wiki submodule"`
- Keep the wiki as the living source of truth for what Nymeria can do today.

### 5. Commit Discipline
- One logical change per commit. Backend and frontend for the same feature can be one commit.
- Reference GitHub issue numbers in commit messages (e.g., `Implement APRS-IS transport (#6)`)
- Run `make test` and `make lint` before every commit.

## Build & Run
- `make build` — builds frontend (pnpm) then Go binary
- `make frontend` / `make backend` — build individually
- `./nymeria --listen :9090` — use port 9090 (8080 often in use)
- `./nymeria --version` — prints version
- `make test` — `go test ./...`
- `make lint` — `go vet ./...`

## Architecture
- Go + SvelteKit single binary via `go:embed`
- Module: `github.com/narvel/nymeria`
- Go 1.24+ required (modernc.org/sqlite)
- Frontend in `web/`, built to `web/build/`, embedded via `web/embed.go`
- Chi v5 router, nhooyr.io/websocket, modernc.org/sqlite, gopkg.in/yaml.v3

## Key Patterns
- Annotations are the unified geographic markup system (no separate "location presets") — `internal/annotation/` handles categories, net-scoping, GPX/KML import, APRS object bridge
- Annotation category metadata lives in `web/src/lib/annotationMeta.ts` (colors, icons, statuses, allowed geometry)
- Transport interface in `internal/transport/transport.go` — all transports implement it
- Stubs return nil/empty — real implementations replace stubs in-place
- Server uses chi middleware (Logger, Recoverer)
- SPA fallback: unmatched routes serve `index.html`
- Config defaults in `internal/config/config.go` DefaultConfig()
- All Go structs that serialize to JSON for the frontend MUST have `json:"camelCase"` tags — Go defaults to PascalCase which silently mismatches TypeScript interfaces
- SQLite: `db.SetMaxOpenConns(1)` and `PRAGMA busy_timeout=5000` are set in `sqlite.go` Init() — do not remove these or SQLITE_BUSY errors will occur under load
- JSON slice columns (TrackedStations, MissionIDs, PinnedStations): `[]string` type, `'[]'` SQL default, marshal on save, unmarshal on load, never use pointer type

## GitHub
- Repo: longjos/Nymeria
- Issues #1-#5 are phase epics; #6-#44 are detailed sub-issues
- Labels: phase:1-core through phase:5-advanced, backend, frontend, protocol, transport, epic

## Gotchas
- `gh issue view N` fails with GraphQL error (Projects Classic deprecation) — always use `gh issue view N --json title,body,state`
- Port 8080 typically in use on this workstation — always test with `--listen :9090`
- pnpm 10.x `approve-builds` warning for esbuild is benign, doesn't block builds
- `web/build/` must exist before Go compiles (go:embed directive)
- Wiki submodule in `wiki/` has the full project spec
- SQLite schema version: after bumping `currentSchemaVersion` in `sqlite.go`, update ALL assertions in `sqlite_test.go` that check the version number
- Weather data flows through existing station events — WebSocket broadcasts include it automatically, no separate weather WebSocket messages needed
- Wiki submodule push order: push `wiki/` submodule first (`cd wiki && git push origin master`), then push main repo — `git push origin main` fails if submodule ref doesn't exist on remote
- Svelte 5: `<button>` inside `<button>` is a build error — use `<div role="button" tabindex="0">` for outer interactive containers that need inner buttons
