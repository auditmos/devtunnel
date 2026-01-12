# DevTunnel

Open-source ngrok alternative. Single Go binary, zero deps, instant setup.

## What

**Tech Stack:**
- Go (Golang) - cross-compilation
- WebSocket + Yamux - multiplexed tunneling
- SQLite (modernc.org/sqlite) - pure Go, embedded
- HTMX - server-rendered dashboard

**Project Structure (Domain-driven):**
```
cmd/
  devtunnel/         # main entrypoint
tunnel/              # WebSocket tunnel, Yamux mux
dashboard/           # HTMX UI, localhost:4040
storage/             # SQLite repos (tunnels, requests, scrub_rules)
proxy/               # HTTP proxy, request interception
crypto/              # AES-GCM encryption for zero-knowledge sharing
server/              # Public gateway mode (subdomain routing, ACME)
api/                 # JSON API for AI agents
```

## Why

Solves webhook debugging for distributed teams:
1. Dev A sees bug -> clicks "Share Securely"
2. Request encrypted client-side (AES-GCM), key in URL hash (server never sees)
3. Dev B clicks link -> decrypts in browser -> replays to their localhost

Two modes, same binary:
- **Client:** `devtunnel start 3000` - exposes localhost, dashboard at :4040
- **Server:** `devtunnel server` - public gateway, auto-HTTPS

## How

### Build & Run
```bash
go build -o devtunnel ./cmd/devtunnel
./devtunnel start 3000        # client mode
./devtunnel server            # server mode
```

### Test
```bash
go test ./...                 # all tests
go test -v ./tunnel/...       # specific package
```

### Lint
```bash
golangci-lint run
```

## Key Libraries
- CLI: `urfave/cli/v2`
- WebSocket: `gorilla/websocket`
- Multiplexing: `hashicorp/yamux`
- IDs: `oklog/ulid`
- Testing: `stretchr/testify`

## MVP Decisions
- **Auth:** Anonymous only (no token required)
- **Rate limiting:** None
- **Blob expiration:** Fixed 24h
- **Subdomains:** Free for all, random, released on disconnect

## Product Requirements
- `.docs/roadmap/prd.json` - **MVP tasks.** Current sprint. Check `passes` field for completion status.
- `.docs/roadmap/prd_future.json` - **Post-MVP features.** Auth, rate limiting, billing, reserved subdomains. Do not implement unless explicitly requested.

## Agent Docs (Read as needed)
- `.docs/agent/001_building_the_project.md` - build commands, cross-compile, Docker
- `.docs/agent/002_running_tests.md` - test patterns, coverage, mocking
- `.docs/agent/003_code_conventions.md` - Go style, error handling, naming
- `.docs/agent/004_service_architecture.md` - component design, data flow
- `.docs/agent/005_database_schema.md` - SQLite tables, migrations
- `.docs/agent/006_service_communication_patterns.md` - WebSocket protocol, framing

## Critical Patterns

### Error Handling
Wrap errors with context: `fmt.Errorf("tunnel connect: %w", err)`

### No `any` Type
Always define interfaces. Never use `any` or `interface{}`.

### Comments
Only for complex logic. Code should be self-documenting.

### Security
- `--safe` flag scrubs sensitive headers before logging
- Zero-knowledge: encryption key never leaves client (URL hash)
- Validate all external input at boundaries
