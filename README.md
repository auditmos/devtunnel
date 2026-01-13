# DevTunnel

**A "not-stupid", open-source alternative to ngrok.**
Built with a "Product, not Infrastructure" philosophy. Single binary, zero dependencies, instant setup.

## 🚀 Philosophy
- **Zero Config:** No `node_modules`, no external DB, no Webpack.
- **Instant Setup:** Download, run, done (< 5 minutes).
- **AI-Native:** JSON logs and API designed for Agent consumption.
- **Self-Contained:** SQLite embedded within the binary.

## 📦 Installation

### Client (Localhost)
```bash
# Standard
curl -sSL https://raw.githubusercontent.com/auditmos/devtunnel/main/scripts/install.sh | sh

# macOS (Homebrew)
brew tap auditmos/devtunnel
brew install devtunnel

devtunnel start 3000
```
**Dashboard:** `http://localhost:4040` (Human) | `http://localhost:4040/api/requests` (AI Agent)

### Server (VPS)
```bash
# Docker (Recommended)
docker run -d -p 80:80 -p 443:443 ghcr.io/auditmos/devtunnel server

# Binary
curl -sSL https://raw.githubusercontent.com/auditmos/devtunnel/main/scripts/install.sh | sh
devtunnel server
```
**Done.** Auto-HTTPS, auto-domain, zero config.

### Manual Download
```bash
# Download for your platform from releases
wget https://github.com/auditmos/devtunnel/releases/latest/download/devtunnel-$(uname -s)-$(uname -m)
chmod +x devtunnel-*
./devtunnel-* start 3000
```

## 🛠 Tech Stack (The "Not-Stupid" Stack)
- **Language:** Go (Golang) - Easy cross-compilation.
- **Transport:** WebSocket + Yamux (Multiplexing) - Bypasses firewalls, handles concurrency.
- **Storage:** SQLite (pure Go via `modernc.org/sqlite`).
- **UI:** Go Templates + HTMX. Server-side rendered dashboard embedded in the binary.

## ⚙️ Architecture & Workflow

### 1. Client Mode (Localhost)
Expose your local app and inspect traffic.
```bash
./devtunnel start 3000 --safe
```
- **Proxy:** Forwards traffic to your app on port 3000.
- **Dashboard:** GUI at `localhost:4040` (HTMX) to view logs.
- **Interception:** Logs requests to embedded SQLite.
- **Replay:** One-click request replay from the dashboard.
- **Security:** `--safe` flag scrubs sensitive headers (defined in `scrub_rules`).

### 2. Server Mode (VPS)
The public gateway.
```bash
./devtunnel server
```
- Accepts WebSocket connections.
- Routes subdomains (`*.devtunnel.me`) to client sockets.
- Auto-HTTPS via Let's Encrypt.

## 💾 Data Model (SQLite)
Simple 3-table schema for maximum efficiency:
1.  **`tunnels`**: Session history.
2.  **`requests`**: Full request/response log for "Webhook Replay".
3.  **`scrub_rules`**: Security patterns to redact (e.g., API keys).

## 📚 Key Libraries
- **CLI:** `urfave/cli/v2`
- **WS:** `gorilla/websocket`
- **Mux:** `hashicorp/yamux`
- **ID:** `oklog/ulid`
- **UI:** `htmx.org`
