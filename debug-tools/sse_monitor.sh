#!/bin/bash

# SSE Connection Monitor for Remote MCP Proxy
# Usage: ./sse_monitor.sh [server_name] [auth_token]

SERVER=${1:-memory}
TOKEN=${2:-test123}
DOMAIN=${DOMAIN:-mcp.home.pezzos.com}

echo "=== Remote MCP SSE Monitor ==="
echo "Server: $SERVER"
echo "Domain: $DOMAIN"
echo "Token: ${TOKEN:0:10}..."
echo "============================="

echo ""
echo "Testing SSE Connection:"
echo "curl -H 'Authorization: Bearer $TOKEN' 'https://$DOMAIN/$SERVER/sse'"
echo ""

# Monitor SSE connection with timeout
timeout 10s curl -H "Authorization: Bearer $TOKEN" \
    -H "Accept: text/event-stream" \
    -H "Cache-Control: no-cache" \
    "https://$DOMAIN/$SERVER/sse" \
    -v 2>&1 | while IFS= read -r line; do
    echo "[$(date '+%H:%M:%S')] $line"
done

echo ""
echo "SSE Monitor completed (10s timeout)"