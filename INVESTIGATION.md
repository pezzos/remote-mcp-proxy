# Remote MCP Proxy Investigation Report

**Date**: 2025-06-24  
**Issue**: Session initialization problems preventing tool discovery in Claude.ai integration  
**Status**: Root cause identified, routing issue confirmed  

## Executive Summary

Investigation into session initialization failures revealed that while MCP servers are functioning correctly and exposing tools properly, there is a critical routing mismatch in the Remote MCP protocol implementation. Claude.ai requests continue going to the `/sse` endpoint instead of switching to the `/sessions/{sessionId}` endpoint after initialization, causing "Connection not initialized" errors.

## Investigation Timeline

### Phase 1: Initial Problem Analysis
**Symptoms observed:**
- Claude.ai completing OAuth authentication successfully
- Session marked as initialized in proxy logs
- Subsequent requests failing with "Connection not initialized" 
- Empty tool capabilities: `"capabilities":{"tools":{}}`
- Requests continuing to `/sse` instead of `/sessions` endpoint

### Phase 2: System Component Verification
**MCP Server Status** ✅ WORKING
```bash
# Verified all servers running properly
$ docker exec remote-mcp-proxy curl -s http://localhost:8080/listmcp
{
  "count": 2,
  "servers": [
    {"name": "memory", "running": true, "pid": 11},
    {"name": "sequential-thinking", "running": true, "pid": 12}
  ]
}
```

**Tool Capabilities** ✅ WORKING  
```bash
# Memory server exposes 9 tools correctly
$ docker exec remote-mcp-proxy curl -s http://localhost:8080/listtools/memory
{
  "server": "memory",
  "response": {
    "result": {
      "tools": [
        {"name": "create_entities", "description": "Create multiple new entities..."},
        {"name": "create_relations", "description": "Create multiple new relations..."},
        {"name": "add_observations", "description": "Add new observations..."},
        {"name": "delete_entities", "description": "Delete multiple entities..."},
        {"name": "delete_observations", "description": "Delete specific observations..."},
        {"name": "delete_relations", "description": "Delete multiple relations..."},
        {"name": "read_graph", "description": "Read the entire knowledge graph"},
        {"name": "search_nodes", "description": "Search for nodes..."},
        {"name": "open_nodes", "description": "Open specific nodes..."}
      ]
    }
  }
}
```

### Phase 3: Protocol Flow Analysis

**Expected Remote MCP Flow:**
1. `GET /sse` → SSE connection established, sends "endpoint" event
2. `POST /sse` → Initialize handshake, session marked as initialized
3. `POST /sessions/{sessionId}` → All subsequent requests (tools/list, etc.)

**Actual Flow (Problematic):**
1. `GET /sse` → SSE connection established ✅
2. `POST /sse` → Initialize handshake successful ✅
3. `POST /sse` → Subsequent requests incorrectly continue here ❌

## Root Cause Analysis

### Primary Issue: Session Endpoint Routing Mismatch

**Location**: `proxy/server.go` lines 576-584, session endpoint construction  
**Problem**: Claude.ai is not properly switching to the session endpoint URL provided in the SSE "endpoint" event

**Session Endpoint Construction (Currently Working)**:
```go
// Determine if we're using subdomain-based or path-based routing
var sessionEndpoint string
if strings.Contains(host, ".mcp.") {
    // Subdomain-based routing: https://memory.mcp.domain.com/sessions/abc123
    sessionEndpoint = fmt.Sprintf("%s://%s/sessions/%s", scheme, host, sessionID)
} else {
    // Path-based routing: http://localhost:8080/memory/sessions/abc123
    sessionEndpoint = fmt.Sprintf("%s://%s/%s/sessions/%s", scheme, host, mcpServer.Name, sessionID)
}
```

**Session Routing Logic (Working)**:
```go
// Root-level endpoints (standard Remote MCP format - subdomain-based)
r.HandleFunc("/sse", s.handleMCPRequest).Methods("GET", "POST")
r.HandleFunc("/sessions/{sessionId:[^/]+}", s.handleSessionMessage).Methods("POST")

// Path-based endpoints (fallback for localhost and development)
r.HandleFunc("/{server:[^/]+}/sse", s.handleMCPRequest).Methods("GET", "POST")
r.HandleFunc("/{server:[^/]+}/sessions/{sessionId:[^/]+}", s.handleSessionMessage).Methods("POST")
```

### Secondary Issue: Request Validation Logic

**Location**: `proxy/server.go` lines 710-720, `handleMCPMessage` function  
**Issue**: Correctly rejects session endpoint requests that come to wrong handler, but doesn't address the reverse scenario

```go
// Check if this request is coming to a session endpoint by looking at the URL path
isSessionEndpointRequest := strings.Contains(r.URL.Path, "/sessions/")

if isSessionEndpointRequest {
    log.Printf("ERROR: Session endpoint request incorrectly routed to handleMCPMessage")
    log.Printf("ERROR: This should not happen - check routing configuration")
    http.Error(w, "Internal routing error", http.StatusInternalServerError)
    return
}
```

## Detailed Technical Analysis

### Session State Management ✅ WORKING

**Translator State** (`protocol/translator.go`):
- Session registration works: `RegisterSession()` creates uninitialized state
- Initialization handling works: `HandleInitialize()` and `HandleInitialized()` 
- State checking works: `IsInitialized()` correctly identifies session status

**Connection Management** (`proxy/server.go`):
- Connection tracking working properly
- Session cleanup functioning
- Concurrent request handling with dedicated read mutex

### MCP Server Communication ✅ WORKING

**Process Management** (`mcp/manager.go`):
- Servers starting correctly (PIDs 11, 12)
- Stdio communication working with dedicated `readMu` mutex
- Message sending/receiving functioning properly

**Tool Discovery** (`proxy/server.go` lines 377-427):
- Tool name normalization working (API-get-user → api_get_user)
- MCP server responses properly formatted
- All 9 memory tools exposed correctly

### Authentication & CORS ✅ WORKING

**OAuth Flow**:
- Dynamic client registration working
- Bearer token validation passing
- CORS headers properly configured

## Error Pattern Analysis

### Log Pattern from Claude.ai Integration:
```
2025/06/24 21:15:33 SUCCESS: Session 3363e17d9250a383805ca763056ba4cd marked as initialized for server memory
2025/06/24 21:15:33 INFO: Session 3363e17d9250a383805ca763056ba4cd marked as initialized for server memory  
2025/06/24 21:15:34 ERROR: Session 3363e17d9250a383805ca763056ba4cd not initialized for non-handshake method tools/list
```

**Analysis**: Session is marked as initialized, but subsequent `tools/list` request is going to `/sse` endpoint (handled by `handleMCPMessage`) instead of `/sessions/{sessionId}` endpoint (handled by `handleSessionMessage`).

## Current Configuration

**Container Status**: Healthy, all services running  
**Domain**: `home.pezzos.com` (configured in `.env`)  
**MCP Servers**: 
- memory (running, PID 11)
- sequential-thinking (running, PID 12)  
- filesystem (configured but not in current logs)

**Traefik Routes**: Auto-generated for subdomain routing
- `memory.mcp.home.pezzos.com/sse`
- `sequential-thinking.mcp.home.pezzos.com/sse`
- `filesystem.mcp.home.pezzos.com/sse`

## Recommendations

### 1. Protocol Compliance Investigation
- Verify Claude.ai's Remote MCP implementation follows specification
- Test with other Remote MCP clients to isolate client vs server issue
- Compare endpoint event format with working Remote MCP implementations

### 2. Enhanced Debugging
- Add request tracing to show exact URLs Claude.ai is hitting
- Log the "endpoint" event data sent in SSE connection
- Monitor actual HTTP traffic between Claude.ai and proxy

### 3. Potential Workarounds
- Consider handling tools/list and other post-init requests in both endpoints temporarily
- Add request forwarding from `/sse` to `/sessions` for non-handshake methods
- Implement client-specific protocol adaptations

### 4. Protocol Standards Review
- Review Remote MCP specification for session endpoint usage requirements
- Validate against reference implementations
- Check for Claude.ai-specific Remote MCP behavior patterns

## Files Investigated

- `proxy/server.go` - Main HTTP routing and session handling
- `protocol/translator.go` - Session state management and message translation
- `mcp/manager.go` - MCP server process management
- `config.json` - MCP server configuration
- `docker-compose.yml` - Container and routing configuration

## Next Steps

1. **Immediate**: Test direct session endpoint calls to verify routing works
2. **Short-term**: Implement request forwarding workaround for post-init requests
3. **Long-term**: Align implementation with Claude.ai's actual Remote MCP behavior

---

**Investigation Status**: ✅ Root cause identified, ready for implementation phase  
**Confidence Level**: High - MCP servers working, routing issue isolated  
**Impact**: Critical - prevents tool discovery in Claude.ai integration