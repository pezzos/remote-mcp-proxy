#!/bin/bash

# Initialize Request Tester for Remote MCP Proxy
# Usage: ./test_initialize.sh [server_name] [session_id]

SERVER=${1:-memory}
SESSION=${2:-test-session-$(date +%s)}
TOKEN=${3:-test123}
DOMAIN=${DOMAIN:-mcp.home.pezzos.com}

echo "=== Remote MCP Initialize Tester ==="
echo "Server: $SERVER"
echo "Session: $SESSION"
echo "Domain: $DOMAIN"
echo "=================================="

# Test 1: SSE Connection
echo ""
echo "1. Testing SSE connection establishment:"
timeout 5s curl -H "Authorization: Bearer $TOKEN" \
    "https://$DOMAIN/$SERVER/sse" \
    -v 2>&1 | head -20

echo ""
echo "2. Testing initialize via session endpoint:"

INIT_REQUEST='{
  "jsonrpc": "2.0",
  "id": "init-test-'$(date +%s)'",
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "capabilities": {
      "tools": {
        "listChanged": true
      }
    },
    "clientInfo": {
      "name": "debug-test",
      "version": "1.0"
    }
  }
}'

echo "Sending initialize request:"
echo "$INIT_REQUEST" | jq .

curl -X POST "https://$DOMAIN/$SERVER/sessions/$SESSION" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -H "Mcp-Session-Id: $SESSION" \
    -d "$INIT_REQUEST" \
    -v 2>&1

echo ""
echo "3. Testing tools/list after initialize:"

TOOLS_REQUEST='{
  "jsonrpc": "2.0",
  "id": "tools-test-'$(date +%s)'",
  "method": "tools/list",
  "params": {}
}'

curl -X POST "https://$DOMAIN/$SERVER/sessions/$SESSION" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -H "Mcp-Session-Id: $SESSION" \
    -d "$TOOLS_REQUEST" \
    -v 2>&1

echo ""
echo "Initialize test completed"