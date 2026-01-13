#!/usr/bin/env bash
set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BINARY="$PROJECT_DIR/devtunnel"
SERVER_PORT=8080
CLIENT_PORT=3000
DOMAIN="localhost"

PIDS=()

cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"
    for pid in "${PIDS[@]}"; do
        kill "$pid" 2>/dev/null || true
    done
    wait 2>/dev/null || true
    echo -e "${GREEN}Done${NC}"
}
trap cleanup EXIT

log_pass() { echo -e "${GREEN}✓ $1${NC}"; }
log_fail() { echo -e "${RED}✗ $1${NC}"; FAILED=1; }
log_info() { echo -e "${YELLOW}→ $1${NC}"; }

FAILED=0

# Build
log_info "Building devtunnel..."
cd "$PROJECT_DIR"
go build -o "$BINARY" ./cmd/devtunnel
log_pass "Build succeeded"

# Start test HTTP server
log_info "Starting test server on :$CLIENT_PORT..."
python3 -m http.server "$CLIENT_PORT" --directory "$PROJECT_DIR" >/dev/null 2>&1 &
PIDS+=($!)
sleep 1

# Start devtunnel server
log_info "Starting devtunnel server on :$SERVER_PORT..."
"$BINARY" server --port "$SERVER_PORT" --domain "$DOMAIN" >/dev/null 2>&1 &
PIDS+=($!)
sleep 2

# Start devtunnel client
log_info "Starting devtunnel client..."
CLIENT_OUTPUT=$(mktemp)
"$BINARY" start -p "$CLIENT_PORT" -s "localhost:$SERVER_PORT" > "$CLIENT_OUTPUT" 2>&1 &
PIDS+=($!)
sleep 3

# Extract subdomain from URL like http://a1b2c3d4.localhost
SUBDOMAIN=$(grep -oE 'http://([a-f0-9]{8})\.' "$CLIENT_OUTPUT" | sed 's|http://||;s|\.||' | head -1 || true)
if [ -z "$SUBDOMAIN" ]; then
    log_fail "Could not extract subdomain from client output"
    cat "$CLIENT_OUTPUT"
    exit 1
fi
log_pass "Client connected, subdomain: $SUBDOMAIN"

# Test 1: Health endpoint
log_info "Testing /health endpoint..."
RESP=$(curl -sf "http://localhost:$SERVER_PORT/health" || echo "FAIL")
if echo "$RESP" | grep -q '"ok":true'; then
    log_pass "/health returns ok"
else
    log_fail "/health failed: $RESP"
fi

# Test 2: Path-based proxy
log_info "Testing /proxy/$SUBDOMAIN/..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:$SERVER_PORT/proxy/$SUBDOMAIN/")
if [ "$HTTP_CODE" = "200" ]; then
    log_pass "/proxy/$SUBDOMAIN/ returns 200"
else
    log_fail "/proxy/$SUBDOMAIN/ returned $HTTP_CODE"
fi

# Test 3: Subdomain routing via Host header
log_info "Testing subdomain routing via Host header..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "Host: $SUBDOMAIN.$DOMAIN" "http://localhost:$SERVER_PORT/")
if [ "$HTTP_CODE" = "200" ]; then
    log_pass "Host: $SUBDOMAIN.$DOMAIN returns 200"
else
    log_fail "Host header routing returned $HTTP_CODE"
fi

# Test 4: Dashboard API
log_info "Testing dashboard /api/requests..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:4040/api/requests")
if [ "$HTTP_CODE" = "200" ]; then
    log_pass "Dashboard /api/requests returns 200"
else
    log_fail "Dashboard API returned $HTTP_CODE"
fi

# Test 5: Invalid subdomain returns 502
log_info "Testing invalid subdomain..."
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -H "Host: nonexistent.$DOMAIN" "http://localhost:$SERVER_PORT/")
if [ "$HTTP_CODE" = "502" ]; then
    log_pass "Invalid subdomain returns 502"
else
    log_fail "Invalid subdomain returned $HTTP_CODE (expected 502)"
fi

# Test 6: --safe flag scrubs sensitive headers
log_info "Testing --safe flag header scrubbing..."
# Kill current client, restart with --safe
kill "${PIDS[2]}" 2>/dev/null || true
sleep 1
SAFE_CLIENT_OUTPUT=$(mktemp)
"$BINARY" start --safe -p "$CLIENT_PORT" -s "localhost:$SERVER_PORT" --dashboard-addr "127.0.0.1:4041" > "$SAFE_CLIENT_OUTPUT" 2>&1 &
PIDS+=($!)
sleep 3

SAFE_SUBDOMAIN=$(grep -oE 'http://([a-f0-9]{8})\.' "$SAFE_CLIENT_OUTPUT" | sed 's|http://||;s|\.||' | head -1 || true)
if [ -z "$SAFE_SUBDOMAIN" ]; then
    log_fail "Could not start safe-mode client"
else
    # Send request with sensitive header
    curl -s -H "Authorization: Bearer secret-token-123" -H "Host: $SAFE_SUBDOMAIN.$DOMAIN" "http://localhost:$SERVER_PORT/" >/dev/null
    sleep 1
    # Check dashboard for scrubbed header (Authorization should be ***)
    REQUESTS=$(curl -sf "http://127.0.0.1:4041/api/requests?limit=1" || echo "")
    if echo "$REQUESTS" | grep -q '"Authorization":"\*\*\*"'; then
        log_pass "--safe flag scrubs Authorization header"
    else
        log_fail "--safe flag did not scrub headers"
        echo "Response: $REQUESTS" | head -c 500
    fi
fi
rm -f "$SAFE_CLIENT_OUTPUT"

# Test 7: --json flag outputs JSONL to stdout
log_info "Testing --json flag JSONL output..."
# Kill current client, restart with --json
kill "${PIDS[2]}" 2>/dev/null || true
sleep 1
JSON_CLIENT_OUTPUT=$(mktemp)
"$BINARY" start --json -p "$CLIENT_PORT" -s "localhost:$SERVER_PORT" --dashboard-addr "127.0.0.1:4042" > "$JSON_CLIENT_OUTPUT" 2>&1 &
PIDS+=($!)
sleep 3

JSON_SUBDOMAIN=$(grep -oE 'http://([a-f0-9]{8})\.' "$JSON_CLIENT_OUTPUT" | sed 's|http://||;s|\.||' | head -1 || true)
if [ -z "$JSON_SUBDOMAIN" ]; then
    log_fail "Could not start json-mode client"
else
    # Send a request to generate log output
    curl -s -H "Host: $JSON_SUBDOMAIN.$DOMAIN" "http://localhost:$SERVER_PORT/" >/dev/null
    sleep 1
    
    # Extract JSONL entries (lines starting with {)
    JSON_LINES=$(grep -E '^\{' "$JSON_CLIENT_OUTPUT" | head -1 || echo "")
    if [ -n "$JSON_LINES" ]; then
        # Validate it's valid JSON by trying to parse it
        if echo "$JSON_LINES" | python3 -m json.tool >/dev/null 2>&1; then
            # Check for required fields
            if echo "$JSON_LINES" | grep -q '"method"' && \
               echo "$JSON_LINES" | grep -q '"url"' && \
               echo "$JSON_LINES" | grep -q '"timestamp"'; then
                log_pass "--json flag outputs valid JSONL"
            else
                log_fail "--json flag output missing required fields"
            fi
        else
            log_fail "--json flag output is not valid JSON"
            echo "Invalid JSON line: $JSON_LINES"
        fi
    else
        log_fail "--json flag did not output JSONL entries"
        echo "Client output (last 20 lines):"
        tail -20 "$JSON_CLIENT_OUTPUT"
    fi
fi
rm -f "$JSON_CLIENT_OUTPUT"

rm -f "$CLIENT_OUTPUT"

echo ""
if [ "$FAILED" = "0" ]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed${NC}"
    exit 1
fi
