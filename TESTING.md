# DevTunnel Manual Testing Guide

## Testing Prerequisites

### Build
```bash
cd /Users/tkow/Documents/Code/Auditmos/brainstorm/devtunnel
go build -o devtunnel ./cmd/devtunnel
```

### No local databases
```bash
cd ~/.devtunnel
ls -la
```

### Simple Test Server
```bash
# Terminal 1 - local test server
python3 -m http.server 3000
```

## Test Scenarios

### Scenario 1: Dashboard Accessibility

**Purpose:** Verify dashboard starts and is accessible

```bash
# Terminal 1: Start test server
python3 -m http.server 3000

# Terminal 2: Start client (no server yet)
./devtunnel start 3000 

# Expected output:
# Request logging enabled: /Users/tkow/.devtunnel/devtunnel.db
# Dashboard: http://127.0.0.1:4040
# 2026/01/13 11:51:34 connect failed: websocket dial: dial tcp [::1]:8080: connect: connection refused, retry in 1s
# 2026/01/13 11:51:36 connect failed: websocket dial: dial tcp [::1]:8080: connect: connection refused, retry in 2s
```

**Additional Options**

- JSON output: Use the `--json` flag to also output logs to stdout in JSONL format:
  ```bash
  ./devtunnel start 3000 --json
  ```

- Safe mode: Use `--safe` to scrub sensitive headers (like Authorization) before logging:
  ```bash
  ./devtunnel start 3000 --safe
  ```

**Tests:**
1. Open http://localhost:4040 in browser
   - ✓ Should show dashboard UI with "No requests yet" or similar
   - ✗ If unreachable: dashboard server didn't start

2. Check port binding:
   ```bash
   lsof -i :4040
   # Should show devtunnel process
   ```

3. Test API endpoint:
   ```bash
   curl http://localhost:4040/api/requests
   # Should return: {"requests":[]}
   ```
4. Check if devtunnel client database is created
   ```bash
   ls -la
   total 64
   # Excpected output:
   # drwxr-xr-x@  3 tkow  staff     96 13 sty 11:56 .
   # drwxr-x---+ 78 tkow  staff   2496 13 sty 11:56 ..
   # -rw-r--r--@  1 tkow  staff  32768 13 sty 11:56 devtunnel.db
   ```
5. Open the database
   ```bash
   sqlite3 ~/.devtunnel/devtunnel.db
   sqlite> select count(*) from requests;
   0
   ```

**Debug if fails:**
- Check firewall blocking port 4040
- Try explicit IPv4: `curl http://127.0.0.1:4040`
- Check logs for dashboard errors

---

### Scenario 2: Server-Client Connection

**Purpose:** Verify WebSocket connection and yamux session establishment

```bash
# Terminal 1: Server
./devtunnel server --port 8080 --domain localhost

# Expected output:
# Server ready on [::]:8080
# Server listening on :8080
# Public URL: http://*.localhost

# Terminal 2: Client
./devtunnel start 3000 --safe --server localhost:8080

# Expected successful output:
# Request logging enabled: /Users/tkow/.devtunnel/devtunnel.db
# Dashboard: http://[::]:4040
# Connected: http://XXXXX.localhost
# Forwarding http://XXXXX.localhost -> localhost:3000

# Server should show:
# Client connected: XXXXX -> http://XXXXX.localhost
```

**Tests:**

1. Connection established (no yamux errors)
   - ✓ Both show connected
   - ✗ If yamux error: handshake stream closed prematurely

2. Session persists:
   ```bash
   # Wait 5 seconds
   # Both should still show connected (no disconnect messages)
   ```

3. Graceful shutdown:
   ```bash
   # In client terminal: Ctrl+C
   # Server should show: Client disconnected: XXXXX
   ```
---

### Scenario 3: HTTP Request Routing

**Purpose:** Demonstrate subdomain routing failure

```bash
./devtunnel server --port 8080 --domain localhost
```

**Tests**

1. Open http://localhost:8080/health in browser
   - ✓ Should show `ok`
   - ✗ If unreachable: server didn't start

2. Check port binding:
   ```bash
   lsof -i :8080
   # Should show devtunnel process
   ```

3. Test API endpoint:
   ```bash
   curl http://localhost:8080/health
   # Should return: ok
   ```
4. Check if devtunnel server database is created
   ```bash
   ls -la
   total 64
   # Excpected output:
   # drwxr-xr-x@  3 tkow  staff     96 13 sty 11:56 .
   # drwxr-x---+ 78 tkow  staff   2496 13 sty 11:56 ..
   # -rw-r--r--@  1 tkow  staff  32768 13 sty 11:56 server.db
   ```
5. Open the database
   ```bash
   sqlite3 ~/.devtunnel/server.db
   sqlite> select count(*) from requests;
   0
   sqlite> select count(*) from tunnels;
   0
   sqlite> select count(*) from shared_blobs;
   0
   ```

```bash
# Setup: Server on 8080, client forwarding 3000, subdomain is 1b4cdd38

# Test 1: Direct Host header (your current approach)
curl -H "Host: 1b4cdd38.localhost" http://localhost:8080

# Expected: ::ffff:127.0.0.1 - - [13/Jan/2026 12:40:23] "GET / HTTP/1.1" 200 -
# In the python3 -m http.server 3000

# Test 2: Using /proxy/ path
curl http://localhost:8080/proxy/1b4cdd38/

# Expected: ::ffff:127.0.0.1 - - [13/Jan/2026 12:40:23] "GET / HTTP/1.1" 200 -
# In the python3 -m http.server 3000

# Test 3: Malformed Host header (your error)
curl -H "Host: http://1b4cdd38.localhost" http://localhost:8080

# Expected: 
```

---

### Scenario 4: Path-Based Routing (Workaround)

**Purpose:** Test existing `/proxy/` endpoint

```bash
# Get your subdomain from client output (e.g., e33ca14d)
SUBDOMAIN=e33ca14d

# Test 1: Root path
curl http://localhost:8080/proxy/$SUBDOMAIN/

# Expected: HTML from python server or test server response

# Test 2: Specific path
curl http://localhost:8080/proxy/$SUBDOMAIN/test/path

# Expected: 404 from local server (path doesn't exist)

# Test 3: With query params
curl "http://localhost:8080/proxy/$SUBDOMAIN/?foo=bar"

# Expected: Response from local server
```

**If this works:** Tunnel logic is functional, only subdomain routing missing

---

### Scenario 5: Dashboard Request Inspection

**Purpose:** Verify logging and dashboard display

```bash
# Setup: Running client with dashboard

# 1. Make request via /proxy/
curl http://localhost:8080/proxy/$SUBDOMAIN/

# 2. Check dashboard
open http://localhost:4040
# Or: curl http://localhost:4040/api/requests | jq

# Expected:
# - Request appears in dashboard
# - Method, URL, status, timing visible
# - Request/response headers and body logged
```

**Tests:**
1. POST request with body:
   ```bash
   curl -X POST -H "Content-Type: application/json" \
     -d '{"test":"data"}' \
     http://localhost:8080/proxy/$SUBDOMAIN/api/test
   ```

2. Verify request body logged in dashboard

3. Test replay feature:
   - Click replay button in dashboard
   - Should re-send to localhost:3000

---

### Scenario 6: Safe Mode Header Scrubbing

**Purpose:** Verify --safe flag scrubs sensitive headers

```bash
# Terminal 1: Client with --safe
./devtunnel start 3000 --safe --server localhost:8080

# Terminal 2: Send request with sensitive headers
curl -H "Authorization: Bearer secret-token" \
     -H "Cookie: session=xyz123" \
     -H "X-API-Key: should-be-hidden" \
     http://localhost:8080/proxy/$SUBDOMAIN/

# Check dashboard at http://localhost:4040
# Expected:
# - Authorization: [REDACTED]
# - Cookie: [REDACTED]
# - X-API-Key: [REDACTED]
```

---

### Scenario 7: JSON Logging Output

**Purpose:** Verify --json flag outputs JSONL to stdout

```bash
./devtunnel start 3000 --safe --json --server localhost:8080 > logs.jsonl

# In another terminal:
curl http://localhost:8080/proxy/$SUBDOMAIN/test

# Check logs.jsonl
cat logs.jsonl | jq

# Expected: JSON objects with request/response data
```

---

### Scenario 8: Connection Resilience

**Purpose:** Test reconnection on server restart

```bash
# Terminal 1: Start server
./devtunnel server --port 8080 --domain localhost

# Terminal 2: Start client
./devtunnel start 3000 --server localhost:8080

# Wait for connection
# Terminal 1: Stop server (Ctrl+C)

# Client should show:
# Connection lost, reconnecting...
# connect failed: ..., retry in 1s
# connect failed: ..., retry in 2s

# Terminal 1: Restart server
./devtunnel server --port 8080 --domain localhost

# Client should reconnect:
# Connected: http://XXXXX.localhost
```

---