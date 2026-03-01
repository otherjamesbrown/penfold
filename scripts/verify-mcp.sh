#!/bin/zsh
#
# Verify MCP server is running on dev02 and tools work.
# Uses curl to send Streamable HTTP MCP requests.
#
# Usage:
#   ./scripts/verify-mcp.sh              # Verify against dev02
#   MCP_URL=http://localhost:50056 ./scripts/verify-mcp.sh  # Verify locally
#

set -e

MCP_URL="${MCP_URL:-http://dev02.brown.chat:50056}"
PASS=0
FAIL=0

green() { echo "\033[0;32m$1\033[0m"; }
red() { echo "\033[0;31m$1\033[0m"; }
cyan() { echo "\033[0;36m$1\033[0m"; }

check() {
    local name="$1"
    local result="$2"
    if [[ $result -eq 0 ]]; then
        green "  PASS: ${name}"
        PASS=$((PASS + 1))
    else
        red "  FAIL: ${name}"
        FAIL=$((FAIL + 1))
    fi
}

# Helper: send an MCP JSON-RPC request and capture the response
mcp_request() {
    local method="$1"
    local params="$2"
    local id="${3:-1}"

    local body="{\"jsonrpc\":\"2.0\",\"id\":${id},\"method\":\"${method}\",\"params\":${params}}"

    curl -sf -X POST "${MCP_URL}/mcp" \
        -H "Content-Type: application/json" \
        -H "Accept: application/json, text/event-stream" \
        -d "$body" 2>/dev/null
}

# Helper: extract result from SSE stream or JSON response
extract_result() {
    # mcp-go Streamable HTTP may return SSE or direct JSON
    # Handle both: look for "result" in the response
    local input="$1"
    # If it's SSE, extract the data line
    if echo "$input" | grep -q "^data:"; then
        echo "$input" | grep "^data:" | sed 's/^data: //' | head -1
    else
        echo "$input"
    fi
}

echo ""
cyan "=== Penfold MCP Server Verification ==="
echo "Target: ${MCP_URL}"
echo ""

# --- 1. Initialize ---
cyan "1. MCP Initialize"
INIT_RESP=$(mcp_request "initialize" '{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"verify","version":"1.0"}}')
INIT_JSON=$(extract_result "$INIT_RESP")

if echo "$INIT_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); assert 'result' in d" 2>/dev/null; then
    check "Initialize returns result" 0

    # Check server name
    SERVER_NAME=$(echo "$INIT_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['result']['serverInfo']['name'])" 2>/dev/null || echo "")
    if [[ "$SERVER_NAME" == "penfold" ]]; then
        check "Server name is 'penfold'" 0
    else
        check "Server name is 'penfold' (got: ${SERVER_NAME})" 1
    fi

    # Check instructions field
    INSTRUCTIONS=$(echo "$INIT_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['result'].get('instructions',''))" 2>/dev/null || echo "")
    if [[ -n "$INSTRUCTIONS" ]]; then
        check "Instructions field present" 0
    else
        check "Instructions field present" 1
    fi
else
    check "Initialize returns result" 1
    check "Server name is 'penfold'" 1
    check "Instructions field present" 1
fi

# Extract session ID from response headers for subsequent requests
SESSION_HEADER=""
# For Streamable HTTP, the session ID comes from the Mcp-Session-Id header
SESSION_ID=$(curl -sf -X POST "${MCP_URL}/mcp" \
    -H "Content-Type: application/json" \
    -H "Accept: application/json, text/event-stream" \
    -d '{"jsonrpc":"2.0","id":99,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"verify2","version":"1.0"}}}' \
    -D - 2>/dev/null | grep -i "mcp-session-id" | sed 's/.*: //' | tr -d '\r\n')

if [[ -n "$SESSION_ID" ]]; then
    SESSION_HEADER="-H Mcp-Session-Id:${SESSION_ID}"

    # Send initialized notification
    curl -sf -X POST "${MCP_URL}/mcp" \
        -H "Content-Type: application/json" \
        -H "Mcp-Session-Id: ${SESSION_ID}" \
        -d '{"jsonrpc":"2.0","method":"notifications/initialized"}' > /dev/null 2>&1 || true
fi

# --- 2. List Tools ---
echo ""
cyan "2. List Tools (startup)"
if [[ -n "$SESSION_ID" ]]; then
    TOOLS_RESP=$(curl -sf -X POST "${MCP_URL}/mcp" \
        -H "Content-Type: application/json" \
        -H "Accept: application/json, text/event-stream" \
        -H "Mcp-Session-Id: ${SESSION_ID}" \
        -d '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' 2>/dev/null)
    TOOLS_JSON=$(extract_result "$TOOLS_RESP")

    TOOL_COUNT=$(echo "$TOOLS_JSON" | python3 -c "import sys,json; print(len(json.load(sys.stdin)['result']['tools']))" 2>/dev/null || echo "0")
    if [[ "$TOOL_COUNT" -eq 5 ]]; then
        check "5 tools at startup (4 meta + health)" 0
    else
        check "5 tools at startup (got: ${TOOL_COUNT})" 1
    fi
else
    check "5 tools at startup (no session)" 1
fi

# --- 3. Call penfold_health ---
echo ""
cyan "3. Health Check"
if [[ -n "$SESSION_ID" ]]; then
    HEALTH_RESP=$(curl -sf -X POST "${MCP_URL}/mcp" \
        -H "Content-Type: application/json" \
        -H "Accept: application/json, text/event-stream" \
        -H "Mcp-Session-Id: ${SESSION_ID}" \
        -d '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"penfold_health","arguments":{}}}' 2>/dev/null)
    HEALTH_JSON=$(extract_result "$HEALTH_RESP")

    if echo "$HEALTH_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); assert 'result' in d" 2>/dev/null; then
        check "penfold_health returns result" 0
    else
        check "penfold_health returns result" 1
    fi
else
    check "penfold_health (no session)" 1
fi

# --- 4. Enable search toolset ---
echo ""
cyan "4. Enable Search Toolset"
if [[ -n "$SESSION_ID" ]]; then
    ENABLE_RESP=$(curl -sf -X POST "${MCP_URL}/mcp" \
        -H "Content-Type: application/json" \
        -H "Accept: application/json, text/event-stream" \
        -H "Mcp-Session-Id: ${SESSION_ID}" \
        -d '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"penfold_enable_toolset","arguments":{"toolset_name":"search"}}}' 2>/dev/null)
    ENABLE_JSON=$(extract_result "$ENABLE_RESP")

    if echo "$ENABLE_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); assert 'result' in d" 2>/dev/null; then
        check "Enable search toolset" 0
    else
        check "Enable search toolset" 1
    fi

    # Re-list tools — should now have 11 (5 + 6 search)
    TOOLS_RESP2=$(curl -sf -X POST "${MCP_URL}/mcp" \
        -H "Content-Type: application/json" \
        -H "Accept: application/json, text/event-stream" \
        -H "Mcp-Session-Id: ${SESSION_ID}" \
        -d '{"jsonrpc":"2.0","id":5,"method":"tools/list","params":{}}' 2>/dev/null)
    TOOLS_JSON2=$(extract_result "$TOOLS_RESP2")

    TOOL_COUNT2=$(echo "$TOOLS_JSON2" | python3 -c "import sys,json; print(len(json.load(sys.stdin)['result']['tools']))" 2>/dev/null || echo "0")
    if [[ "$TOOL_COUNT2" -eq 11 ]]; then
        check "11 tools after enabling search (5 + 6)" 0
    else
        check "11 tools after enabling search (got: ${TOOL_COUNT2})" 1
    fi
else
    check "Enable search toolset (no session)" 1
    check "11 tools after enabling search (no session)" 1
fi

# --- Summary ---
echo ""
TOTAL=$((PASS + FAIL))
if [[ $FAIL -eq 0 ]]; then
    green "=== All ${TOTAL} checks passed ==="
else
    red "=== ${FAIL}/${TOTAL} checks failed ==="
    exit 1
fi
