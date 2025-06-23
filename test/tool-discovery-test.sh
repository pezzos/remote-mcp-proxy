#!/bin/bash

# Comprehensive Tool Discovery Test for Remote MCP Proxy
# Tests that all configured MCP servers expose their tools correctly

set -e

echo "🔍 Starting Tool Discovery Test..."
echo "=================================="

# Configuration
PROXY_URL="http://localhost:8080"
EXPECTED_SERVERS=("notionApi" "memory" "sequential-thinking")
EXPECTED_TOOL_COUNTS=(19 9 1)  # Expected number of tools per server

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test results tracking
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

log_test() {
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    if [ "$1" = "PASS" ]; then
        echo -e "${GREEN}✅ PASS${NC}: $2"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    elif [ "$1" = "FAIL" ]; then
        echo -e "${RED}❌ FAIL${NC}: $2"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    elif [ "$1" = "INFO" ]; then
        echo -e "${BLUE}ℹ️  INFO${NC}: $2"
    elif [ "$1" = "WARN" ]; then
        echo -e "${YELLOW}⚠️  WARN${NC}: $2"
    fi
}

# Test 1: Check if proxy is running
echo -e "\n${BLUE}Test 1: Proxy Health Check${NC}"
if curl -s "$PROXY_URL/health" | grep -q "healthy"; then
    log_test "PASS" "Proxy is running and healthy"
else
    log_test "FAIL" "Proxy is not responding or unhealthy"
    exit 1
fi

# Test 2: List all MCP servers
echo -e "\n${BLUE}Test 2: MCP Server Discovery${NC}"
SERVER_RESPONSE=$(curl -s "$PROXY_URL/listmcp")
SERVER_COUNT=$(echo "$SERVER_RESPONSE" | jq -r '.count')

if [ "$SERVER_COUNT" -eq "${#EXPECTED_SERVERS[@]}" ]; then
    log_test "PASS" "Found expected number of servers: $SERVER_COUNT"
else
    log_test "FAIL" "Expected ${#EXPECTED_SERVERS[@]} servers, found $SERVER_COUNT"
fi

# Check if all expected servers are running
for server in "${EXPECTED_SERVERS[@]}"; do
    if echo "$SERVER_RESPONSE" | jq -r '.servers[].name' | grep -q "$server"; then
        RUNNING=$(echo "$SERVER_RESPONSE" | jq -r ".servers[] | select(.name==\"$server\") | .running")
        if [ "$RUNNING" = "true" ]; then
            log_test "PASS" "Server '$server' is running"
        else
            log_test "FAIL" "Server '$server' is not running"
        fi
    else
        log_test "FAIL" "Server '$server' not found in server list"
    fi
done

# Test 3: Tool Discovery for each server
echo -e "\n${BLUE}Test 3: Tool Discovery per Server${NC}"

for i in "${!EXPECTED_SERVERS[@]}"; do
    server="${EXPECTED_SERVERS[$i]}"
    expected_count="${EXPECTED_TOOL_COUNTS[$i]}"
    
    echo -e "\n  ${YELLOW}Testing server: $server${NC}"
    
    # Test tools/list endpoint
    TOOLS_RESPONSE=$(curl -s "$PROXY_URL/listtools/$server")
    
    # Check if request was successful
    if echo "$TOOLS_RESPONSE" | jq -e '.error' > /dev/null 2>&1; then
        ERROR_MSG=$(echo "$TOOLS_RESPONSE" | jq -r '.message')
        log_test "FAIL" "Server '$server' tool discovery failed: $ERROR_MSG"
        continue
    fi
    
    # Count tools
    TOOL_COUNT=$(echo "$TOOLS_RESPONSE" | jq -r '.response.result.tools | length' 2>/dev/null || echo "0")
    
    if [ "$TOOL_COUNT" -eq "$expected_count" ]; then
        log_test "PASS" "Server '$server' exposes $TOOL_COUNT tools (expected: $expected_count)"
    else
        log_test "WARN" "Server '$server' exposes $TOOL_COUNT tools (expected: $expected_count)"
    fi
    
    # Test tool name normalization
    if [ "$TOOL_COUNT" -gt 0 ]; then
        TOOL_NAMES=$(echo "$TOOLS_RESPONSE" | jq -r '.response.result.tools[].name' 2>/dev/null)
        
        # Check if tool names are properly formatted for Claude.ai
        NORMALIZED_COUNT=0
        TOTAL_TOOL_COUNT=0
        
        while IFS= read -r tool_name; do
            if [ -n "$tool_name" ]; then
                TOTAL_TOOL_COUNT=$((TOTAL_TOOL_COUNT + 1))
                # Check if tool name follows snake_case or acceptable naming convention
                if echo "$tool_name" | grep -qE '^[a-z][a-z0-9_]*$|^[A-Z][A-Za-z0-9_-]*$'; then
                    NORMALIZED_COUNT=$((NORMALIZED_COUNT + 1))
                fi
            fi
        done <<< "$TOOL_NAMES"
        
        if [ "$NORMALIZED_COUNT" -eq "$TOTAL_TOOL_COUNT" ]; then
            log_test "PASS" "Server '$server' all tool names are properly formatted"
        else
            log_test "WARN" "Server '$server' has $((TOTAL_TOOL_COUNT - NORMALIZED_COUNT)) tools with non-standard names"
        fi
        
        # Log a few tool names for debugging
        log_test "INFO" "Sample tools from '$server': $(echo "$TOOL_NAMES" | head -3 | tr '\n' ', ' | sed 's/,$//')"
    fi
done

# Test 4: Remote MCP Integration Test
echo -e "\n${BLUE}Test 4: Remote MCP Protocol Compliance${NC}"

# Test initialize handshake for each server
for server in "${EXPECTED_SERVERS[@]}"; do
    echo -e "\n  ${YELLOW}Testing Remote MCP handshake: $server${NC}"
    
    # Create initialize request
    INIT_REQUEST='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}'
    
    # Test initialize endpoint
    INIT_RESPONSE=$(curl -s -X POST \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer test-token" \
        -d "$INIT_REQUEST" \
        "$PROXY_URL/$server/sse" 2>/dev/null || echo '{"error":"connection_failed"}')
    
    if echo "$INIT_RESPONSE" | jq -e '.result' > /dev/null 2>&1; then
        log_test "PASS" "Server '$server' Remote MCP initialize successful"
        
        # Check if capabilities are returned
        CAPABILITIES=$(echo "$INIT_RESPONSE" | jq -r '.result.capabilities' 2>/dev/null)
        if [ "$CAPABILITIES" != "null" ] && [ "$CAPABILITIES" != "" ]; then
            log_test "PASS" "Server '$server' returns capabilities in initialize response"
        else
            log_test "WARN" "Server '$server' initialize response missing capabilities"
        fi
        
    elif echo "$INIT_RESPONSE" | jq -e '.error' > /dev/null 2>&1; then
        ERROR_CODE=$(echo "$INIT_RESPONSE" | jq -r '.error.code' 2>/dev/null || echo "unknown")
        log_test "WARN" "Server '$server' Remote MCP initialize returned error: $ERROR_CODE"
    else
        log_test "FAIL" "Server '$server' Remote MCP initialize failed or connection error"
    fi
done

# Test Summary
echo -e "\n${BLUE}Test Summary${NC}"
echo "=============="
echo "Total Tests: $TOTAL_TESTS"
echo -e "Passed: ${GREEN}$PASSED_TESTS${NC}"
echo -e "Failed: ${RED}$FAILED_TESTS${NC}"

if [ "$FAILED_TESTS" -eq 0 ]; then
    echo -e "\n${GREEN}🎉 All critical tests passed! Tools should be visible in Claude.ai${NC}"
    exit 0
else
    echo -e "\n${RED}⚠️  Some tests failed. Check the output above for details.${NC}"
    exit 1
fi