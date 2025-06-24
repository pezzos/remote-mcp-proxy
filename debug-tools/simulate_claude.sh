#!/bin/bash

# Simulate Claude.ai Remote MCP Flow
# Usage: ./simulate_claude.sh [server_name]

SERVER=${1:-memory}
TOKEN=${2:-test123}
DOMAIN=${DOMAIN:-mcp.home.pezzos.com}

echo "=== Claude.ai Remote MCP Flow Simulation ==="
echo "Server: $SERVER"
echo "Domain: $DOMAIN"
echo "=========================================="

# Step 1: Establish SSE connection and extract session endpoint
echo ""
echo "Step 1: Establishing SSE connection and extracting session endpoint..."

SSE_OUTPUT=$(timeout 3s curl -s -H "Authorization: Bearer $TOKEN" \
    -H "Accept: text/event-stream" \
    "https://$DOMAIN/$SERVER/sse" 2>/dev/null)

if [ $? -eq 124 ]; then
    echo "✅ SSE connection established (timeout expected)"
else
    echo "❌ SSE connection failed"
    exit 1
fi

# Extract session endpoint from SSE event data
SESSION_ENDPOINT=$(echo "$SSE_OUTPUT" | grep -o '"uri":"[^"]*"' | cut -d'"' -f4)

if [ -z "$SESSION_ENDPOINT" ]; then
    echo "❌ Failed to extract session endpoint from SSE"
    echo "SSE Output: $SSE_OUTPUT"
    exit 1
fi

echo "✅ Session endpoint extracted: $SESSION_ENDPOINT"

# Extract session ID from endpoint URL
SESSION_ID=$(echo "$SESSION_ENDPOINT" | grep -o '/sessions/[^/]*$' | cut -d'/' -f3)
echo "✅ Session ID: $SESSION_ID"

# Step 2: Send initialize request to session endpoint
echo ""
echo "Step 2: Sending initialize request..."

INIT_REQUEST='{
  "jsonrpc": "2.0",
  "id": "init-claude-'$(date +%s)'",
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "capabilities": {
      "tools": {
        "listChanged": true
      }
    },
    "clientInfo": {
      "name": "claude-simulation",
      "version": "1.0"
    }
  }
}'

INIT_RESPONSE=$(curl -s -X POST "$SESSION_ENDPOINT" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -H "Mcp-Session-Id: $SESSION_ID" \
    -d "$INIT_REQUEST")

echo "Initialize response: $INIT_RESPONSE"

# Step 3: Send tools/list request
echo ""
echo "Step 3: Sending tools/list request..."

TOOLS_REQUEST='{
  "jsonrpc": "2.0",
  "id": "tools-claude-'$(date +%s)'",
  "method": "tools/list",
  "params": {}
}'

TOOLS_RESPONSE=$(curl -s -X POST "$SESSION_ENDPOINT" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -H "Mcp-Session-Id: $SESSION_ID" \
    -d "$TOOLS_REQUEST")

echo "Tools response: $TOOLS_RESPONSE"

echo ""
echo "Claude.ai simulation completed"