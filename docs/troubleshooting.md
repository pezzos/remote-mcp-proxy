# Remote MCP Proxy - Troubleshooting Guide

## 🆕 Memory Server Stability Issues - Latest Version ⚠️ **ACTIVE**

### Problem Statement
**Issue**: Memory MCP server becomes unresponsive after initial successful operations, causing timeouts and failed tool calls.

### Root Cause Analysis
**Technical Issue**: Memory MCP server process hangs after handling some requests, becoming completely unresponsive.

**Symptoms**:
1. Initial operations succeed (initialize, tools/list)
2. Server suddenly stops responding to requests
3. 30-second timeouts on all subsequent requests
4. Auto-restart attempts often fail with same pattern
5. "Context deadline exceeded" errors in logs

**Evidence from Logs**:
```
2025/06/26 08:21:20 [INFO] Successfully received response from server memory
2025/06/26 08:21:36 [ERROR] Context cancelled while waiting for response from server memory
2025/06/26 08:22:59 [WARN] MCP server memory appears hung, attempting restart...
```

### 🆕 New Solutions Implemented ✅

**1. Proactive Health Monitoring**:
- **Health Checker**: Continuous monitoring with 30-second ping intervals
- **Early Detection**: 3 consecutive failures trigger automatic recovery
- **Smart Restart**: Graceful server restart with process cleanup
- **Restart Limits**: Maximum 3 restarts per 5-minute window to prevent loops

```bash
# Check real-time health status
curl https://mcp.your-domain.com/health/servers
# Response shows health status, response times, restart counts
```

**2. Resource Monitoring & Alerting**:
- **Process Tracking**: Monitor memory and CPU usage of all MCP processes
- **Alert Thresholds**: Warn on >500MB memory or >80% CPU per process
- **Resource Summaries**: Logged every minute for trend analysis

```bash
# Monitor resource usage
curl https://mcp.your-domain.com/health/resources
# Shows memory/CPU usage, process counts, averages
```

**3. Container Resource Management**:
- **Memory Limits**: 2GB hard limit with 512MB guaranteed reservation
- **CPU Limits**: 2.0 CPU limit with 0.5 CPU guaranteed
- **OOM Prevention**: Prevents resource exhaustion causing hangs

**4. Enhanced Logging & Debugging**:
- **Persistent Logs**: Volume-mounted `/logs` directory
- **Separate Log Files**: System logs + individual MCP server logs
- **Request Correlation**: SessionID included in all log messages
- **Log Retention**: Configurable cleanup (24h system, 12h MCP)

### Manual Troubleshooting Steps

**1. Check Health Status**:
```bash
# Overall system health
curl https://mcp.your-domain.com/health

# Detailed server health
curl https://mcp.your-domain.com/health/servers

# Resource usage
curl https://mcp.your-domain.com/health/resources
```

**2. Review Logs**:
```bash
# System logs
docker exec remote-mcp-proxy tail -f /app/logs/system.log

# Memory server specific logs
docker exec remote-mcp-proxy tail -f /app/logs/mcp-memory.log

# Check for patterns
docker exec remote-mcp-proxy grep "ERROR\|WARN" /app/logs/mcp-memory.log
```

**3. Manual Server Restart** (if needed):
```bash
# The health checker should handle this automatically, but if needed:
# Note: Manual restart endpoints are not exposed for security
# If persistent issues occur, container restart may be needed:
docker-compose restart remote-mcp-proxy
```

### 🔧 Configuration Tuning

**Adjust Health Check Sensitivity** (via environment variables):
```bash
# In .env file - example of more aggressive monitoring
LOG_LEVEL_SYSTEM=DEBUG    # More detailed logging
LOG_LEVEL_MCP=DEBUG       # Detailed MCP server logs
LOG_RETENTION_SYSTEM=48h  # Longer retention for analysis
LOG_RETENTION_MCP=24h     # Longer MCP log retention
```

**Monitor Resource Trends**:
- Check `/health/resources` regularly for memory/CPU patterns
- Look for gradual memory increases (potential leaks)
- Monitor restart frequency in `/health/servers`

### Expected Behavior With New System

**Normal Operation**:
1. Health checker reports all servers as "healthy"
2. Resource usage stays within normal ranges (<500MB/server)
3. Restart count remains low or zero
4. Logs show successful request/response cycles

**During Issues**:
1. Health status changes to "unhealthy" after 3 failed pings
2. Automatic restart attempt logged
3. Resource monitoring may show anomalies before failure
4. Detailed error information captured in server-specific logs

**Recovery**:
1. Successful restart returns status to "healthy"
2. Restart count increments (tracked for pattern analysis)
3. Normal operation resumes automatically
4. If restart limit reached, alerting continues without restart attempts

---

## ✅ RESOLVED: Stale SSE Connection Issue

### Problem Statement (RESOLVED)
**Issue**: Claude.ai showed "connection succeeded" but integration remained "To connect", preventing tool discovery and usage.

### Solution Implemented ✅

**Automatic Stale Connection Detection**:
- **SSE Connection Timeout**: Connections idle for 5+ minutes automatically close
- **Debug Message Limiting**: Only first 10 debug messages per session to prevent log spam
- **Activity Tracking**: Keep-alive events update last activity time
- **Graceful Cleanup**: Automatic resource cleanup on connection termination

**Automatic Background Cleanup**:
- **Periodic Cleanup**: Every 30 seconds, removes connections idle for 2+ minutes
- **Connection Monitoring**: Logs cleanup activity when stale connections are removed
- **Resource Management**: Prevents accumulation of zombie connections

**Manual Cleanup** (if needed):
```bash
# Force cleanup of stale connections
curl -X POST https://mcp.your-domain.com/cleanup
```

---

## Multiple Integration Support ✅ **RESOLVED**

### Problem Statement (RESOLVED)
**Issue**: When multiple Claude.ai integrations were enabled simultaneously, tools from all but one integration would disappear, causing tool discovery conflicts.

### Root Cause Analysis
**Technical Issue**: Single MCP server process serving multiple concurrent sessions without proper request/response correlation, leading to stdout conflicts and response mismatching.

**Specific Problem**:
1. Multiple sessions accessing same MCP server simultaneously
2. Concurrent `tools/list` requests causing response mixing
3. Only "last successful" integration's tools remained visible
4. Other integrations showed empty tool lists

### Solution Implemented ✅
**Request Serialization Architecture**:
- **Per-server request queues**: Each MCP server processes requests one at a time
- **Atomic request/response**: New `SendAndReceive()` method ensures proper correlation
- **Connection isolation**: Sessions maintain separate tool discovery without interference
- **Concurrent server support**: Different servers (memory vs sequential-thinking) still process concurrently

### Connection Cleanup Improvements ✅
**Enhanced Disconnect Detection**:
- **Keep-alive messages**: 30-second SSE keep-alive events detect client disconnection
- **Faster cleanup**: Reduced stale connection timeout from 10 minutes to 2 minutes  
- **Better context handling**: Background contexts with HTTP request monitoring
- **Manual cleanup**: Added `/cleanup` endpoint for administrative control

### Testing Multiple Integrations
```bash
# Test simultaneous connections work correctly
docker exec remote-mcp-proxy curl -X POST http://localhost:8080/cleanup  # Clear any stale connections
# Connect memory and sequential-thinking integrations simultaneously in Claude.ai
# Verify each shows only its intended server's tools

# Monitor connection status
docker logs remote-mcp-proxy --tail=20
# Should show clean logs without "SSE connection active" spam
```

### Deployment Notes
**Required for Multiple Integration Support**:
1. **Deploy Latest Version**: Ensure container includes connection cleanup and request serialization fixes
2. **Health Check**: Wait for `(healthy)` status before testing: `docker-compose ps`
3. **Clean State**: Use cleanup endpoint if needed: `docker exec remote-mcp-proxy curl -X POST http://localhost:8080/cleanup`

## Historical Issues (Previously Resolved)

### ✅ RESOLVED: Tool Discovery Issue

## Test Results Summary

### ✅ WORKING Components
1. **MCP Server Infrastructure**: All 3 servers (memory, sequential-thinking, notionApi) start successfully
2. **Direct Tool Discovery**: `/listtools/memory` and `/listtools/sequential-thinking` return tools correctly
3. **HTTP Endpoints**: Health checks and server listing work
4. **Traefik Routing**: Domain routing to proxy works correctly
5. **Authentication**: Bearer token validation passes
6. **Tool Normalization**: Snake_case conversion works in translator

### ❌ FAILING Components
1. **Remote MCP SSE Flow**: SSE connections hang indefinitely
2. **Session Initialization**: Initialize requests timeout (30s)
3. **Session Endpoints**: Reject requests with "Session not initialized"
4. **Claude.ai Integration**: Connects but no tools discovered

### 🧪 Tests Performed
```bash
# Direct endpoint tests (WORKING)
curl -s https://mcp.home.pezzos.com/listmcp
curl -s https://mcp.home.pezzos.com/listtools/memory
curl -s https://mcp.home.pezzos.com/listtools/sequential-thinking

# Remote MCP protocol tests (PREVIOUSLY FAILING, NOW FIXED)
curl -H "Authorization: Bearer test123" "https://mcp.home.pezzos.com/memory/sse" # Now works
curl -X POST "https://mcp.home.pezzos.com/memory/sse" -H "Authorization: Bearer test123" # Now works
curl -X POST "https://mcp.home.pezzos.com/memory/sessions/test123" # Now works
```

### ✅ RESOLVED: Request Timeout Issue

**Issue**: Tools would appear initially in Claude.ai but then disappear due to request timeouts.

**Root Cause**: After successful `tools/list` response, Claude.ai would send follow-up requests that some MCP servers don't respond to, causing 30-second timeouts and connection cancellation.

**Solution Implemented** (`proxy/server.go:771-804`):
- **Reduced timeout** from 30 to 10 seconds for faster failure detection
- **Fallback response system** for optional Remote MCP methods:
  - `resources/list` → Empty resources array
  - `resources/read` → Method not found error  
  - `prompts/list` → Empty prompts array
  - `prompts/get` → Method not found error
- **Proper error responses** for unsupported methods using JSON-RPC 2.0 format
- **Connection stability** - prevents Claude.ai from canceling connections due to timeouts

This ensures every request gets a response within 10 seconds, maintaining connection stability while preserving tool functionality.

## Root Cause Analysis

### Primary Issue: SSE Connection Deadlock
**Location**: `proxy/server.go:520-623` in `handleSSEConnection`

**Problem**: Infinite loop waits for session initialization but Claude.ai cannot initialize because:
1. SSE connection hangs in message loop
2. Session endpoint requires initialized session 
3. **Chicken-and-egg deadlock**

```go
// Current problematic logic
for {
    if !s.translator.IsInitialized(sessionID) {
        continue  // BLOCKS FOREVER
    }
    // Message processing never reached
}
```

### Secondary Issues

#### 1. MCP Server Communication Timeouts
- **notionApi**: Times out on tools/list (likely API authentication)
- **All servers**: Timeout on initialize requests in Remote MCP flow
- **Working**: Direct /listtools calls succeed

#### 2. Remote MCP Protocol Implementation Gaps
- **Missing**: Proper SSE event streaming before initialization
- **Missing**: Error handling for failed initialization
- **Missing**: Fallback mechanisms for unresponsive servers

## Potential Issues to Investigate

### 🔍 HIGH PRIORITY

#### A. SSE Connection Flow
- [ ] **Fix SSE deadlock**: Remove initialization check from message loop
- [ ] **Test endpoint event**: Verify Claude.ai receives session endpoint URL
- [ ] **Debug SSE headers**: Ensure all required headers are set
- [ ] **Check connection persistence**: Verify SSE connection stays alive

#### B. Initialize Request Handling
- [ ] **Debug initialize timeout**: Increase timeout or fix blocking
- [ ] **Test direct initialize**: Bypass SSE and test POST initialize directly
- [ ] **Check session state**: Verify session creation and storage
- [ ] **Validate initialize response**: Ensure proper JSON-RPC format

#### C. Tool Discovery Protocol
- [ ] **Test tools/list in session**: Send tools/list after successful initialize
- [ ] **Check capability negotiation**: Verify tools capability is advertised
- [ ] **Debug tool normalization**: Ensure Claude.ai receives proper tool format
- [ ] **Validate tool schema**: Check inputSchema compliance

### 🔍 MEDIUM PRIORITY

#### D. MCP Server Stability
- [ ] **Fix notionApi timeouts**: Debug Notion API authentication
- [ ] **Test server restart**: Verify servers recover from failures
- [ ] **Check concurrent access**: Test multiple simultaneous requests
- [ ] **Monitor resource usage**: Check for memory/CPU issues

#### E. Protocol Compliance
- [ ] **Compare with working examples**: Test against Cloudflare MCP server
- [ ] **Validate Remote MCP spec**: Ensure strict protocol compliance
- [ ] **Test with MCP inspector**: Use official tooling for validation
- [ ] **Check error responses**: Verify JSON-RPC error format

#### F. Configuration Issues
- [ ] **Test different MCP servers**: Try filesystem or other simple servers
- [ ] **Check environment variables**: Verify all required env vars
- [ ] **Test minimal config**: Remove complex servers, test with basic ones
- [ ] **Validate JSON config**: Ensure no parsing errors

### 🔍 LOW PRIORITY

#### G. Infrastructure
- [ ] **Docker networking**: Check container communication
- [ ] **Traefik configuration**: Verify reverse proxy settings
- [ ] **SSL/TLS issues**: Check certificate and encryption
- [ ] **CORS configuration**: Verify cross-origin settings

#### H. Logging and Monitoring
- [ ] **Add request tracing**: Implement unique request IDs
- [ ] **Enhanced error reporting**: More detailed error messages
- [ ] **Performance metrics**: Add timing measurements
- [ ] **Connection state tracking**: Monitor session lifecycle

## Investigation Improvements Needed

### 🛠️ Debugging Tools
1. **Real-time SSE monitor**: Tool to watch SSE events as they happen
2. **Session state inspector**: Web interface to view active sessions
3. **MCP message tracer**: Log all MCP communication with timestamps
4. **Protocol validator**: Check Remote MCP compliance automatically

### 🛠️ Test Automation
1. **End-to-end test suite**: Automated Claude.ai connection simulation
2. **MCP server mock**: Simple test server for protocol validation
3. **Performance benchmarks**: Measure request/response times
4. **Load testing**: Simulate multiple concurrent connections

### 🛠️ Development Environment
1. **Local MCP testing**: Test servers outside Docker for easier debugging
2. **Protocol inspection**: Tools to examine JSON-RPC messages
3. **Claude.ai simulator**: Mock client to test Remote MCP flow
4. **Debugging containers**: Add debugging tools to Docker image

## Technical Debt to Clean Up

### 🧹 Unused Code
1. **OAuth endpoints**: Not used by Claude.ai Remote MCP
2. **Legacy headers**: X-Session-ID superseded by Mcp-Session-Id
3. **Unused fallback methods**: resources/list, prompts/list not needed
4. **Debug logging**: Excessive logging in production

### 🧹 Code Improvements
1. **Error handling**: More specific error types and messages
2. **Configuration validation**: Check config on startup
3. **Resource cleanup**: Ensure proper cleanup of failed connections
4. **Documentation**: Add inline documentation for Remote MCP flow

## Current Priority Issues

### ✅ RESOLVED ISSUES
1. ✅ **Automatic stale connection detection**: Implemented with 5-minute timeout
2. ✅ **Connection health monitoring**: Keep-alive tracking and activity monitoring active
3. ✅ **Debug spam prevention**: Limited to 10 messages per session
4. ✅ **Background cleanup**: 30-second interval cleanup of 2+ minute idle connections

### 📋 MONITORING REQUIREMENTS
1. **Watch for stale connection warnings** in logs - should be rare now
2. **Monitor cleanup activity** - automatic removal should handle most issues
3. **Manual cleanup available** if needed: `docker exec remote-mcp-proxy curl -X POST http://localhost:8080/cleanup`

### 📋 COMPLETED ISSUES
1. ✅ **SSE deadlock**: Fixed in handleSSEConnection
2. ✅ **Initialize request flow**: Working correctly
3. ✅ **Remote MCP handshake**: Properly implemented
4. ✅ **Multiple integration support**: Request serialization implemented
5. ✅ **Connection cleanup**: Manual cleanup endpoint functional

## Progress Update - June 24, 05:30 UTC

### ✅ MAJOR BREAKTHROUGHS
1. **SSE Deadlock FIXED**: Removed initialization check from SSE message loop
2. **SSE Connection Working**: Establishes successfully, sends endpoint event
3. **Session Endpoint Routing**: Accepts connections and processes handshake messages
4. **Debug Tools Created**: SSE monitor, initialize tester, Claude simulation

### 🔍 CURRENT ISSUE
**Session Initialize Failing**: Session endpoint receives initialize requests but still returns "Session not initialized"

**Root Cause**: Handshake detection or session state management issue in session endpoint

**Evidence**: 
- SSE connection creates session ID: `3d96bd1605855b4e47a3fbed8d9c5bcd`
- Endpoint event sent with correct session URL
- Session endpoint receives initialize request correctly
- But handshake detection/processing fails

### 💡 **CRITICAL BREAKTHROUGH - REAL ROOT CAUSE DISCOVERED**

After extensive investigation, the **actual problem** is completely different from what we thought:

#### **THE REAL ISSUE: Wrong Server Selection**

**Evidence from Container Logs:**
```
WARNING: ReadMessage timeout/cancellation for server notionApi: context deadline exceeded
(thousands of these messages)
session 2e4d19487d8f47daf622ce1d73ec2c41 for notionApi
```

**What's Actually Happening:**
1. 🔍 **Claude.ai connects to `notionApi` server first** (not `memory` as expected)
2. ❌ **NotionApi server is completely broken** - constant timeouts, never responds
3. 🔄 **SSE loop gets stuck** trying to read from broken notionApi server  
4. 🚫 **Tool discovery fails** because the connection never works

#### **Why We Missed This:**

1. **Wrong Test Assumption**: We tested `/memory/sse` directly, but Claude.ai picks servers from `/listmcp`
2. **Server Order Matters**: NotionApi is listed first in `/listmcp` response:
   ```json
   {"servers": [{"name": "notionApi"}, {"name": "memory"}, {"name": "sequential-thinking"}]}
   ```
3. **Broken Server Breaks Everything**: One bad server ruins the entire integration

#### **Previous Fixes Were Actually Working**: 
- ✅ SSE connections work perfectly  
- ✅ Session registration works
- ✅ Remote MCP protocol is correct
- ❌ **But Claude.ai chooses the broken server**

### 🔧 **IMMEDIATE SOLUTIONS**

#### **Option 1: Disable Broken Server**
```json
{
  "mcpServers": {
    // "notionApi": { ... }, // COMMENT OUT
    "memory": { ... },
    "sequential-thinking": { ... }
  }
}
```

#### **Option 2: Fix Server Order**
Move working servers first in config.json

#### **Option 3: Fix NotionApi Server**
Debug why Notion API authentication is failing

### 🚨 **BREAKTHROUGH #3: DISCOVERED THE FUNDAMENTAL DESIGN FLAW**

**Latest Evidence from Claude.ai Connection Attempt:**
```
session add7b4323b2cf60bd1afaed63e0f5621 for memory server
WARNING: ReadMessage timeout/cancellation for server memory: context deadline exceeded
(thousands of timeout messages)
INFO: SSE connection cleanup completed for server memory, session add7b4323b2cf60bd1afaed63e0f5621
```

**Critical Discovery**: 
🔍 **Claude.ai IS connecting successfully** - SSE session established, endpoint event sent
❌ **SSE loop design is fundamentally wrong** - constantly polling MCP server

### 🧠 **THE REAL PROBLEM: Wrong SSE Loop Logic**

**Current (Broken) Flow:**
1. ✅ Claude.ai connects to SSE endpoint
2. ✅ Session created, endpoint event sent  
3. ❌ **SSE loop immediately starts polling MCP server for messages**
4. ❌ **MCP server has nothing to say until it gets a request**
5. ❌ **Infinite timeout loop blocks everything**

**Correct Remote MCP Flow Should Be:**
1. ✅ Claude.ai connects to SSE endpoint
2. ✅ Session created, endpoint event sent
3. ✅ **SSE loop waits passively**
4. ✅ **Claude.ai sends requests via session endpoint**  
5. ✅ **Only then does proxy read MCP server responses**
6. ✅ **Responses sent back via SSE**

### 💡 **THE ARCHITECTURE PROBLEM**

**Wrong Design:**
```go
// BROKEN: Constantly polling MCP server
for {
    message, err := mcpServer.ReadMessage(ctx) // BLOCKS FOREVER
    // Send via SSE
}
```

**Correct Design:**
```go  
// CORRECT: Event-driven, only read when there's a request
for {
    select {
    case <-ctx.Done(): return
    case request := <-requestChannel:
        // Send request to MCP server
        // Read response from MCP server  
        // Send response via SSE
    }
}
```

### 🔧 **ROOT CAUSE ANALYSIS**

**Why This Wasn't Obvious:**
1. **Claude.ai connects successfully** - appears to work initially
2. **Timeout spam hides the real issue** - looks like server communication problem
3. **No request/response logging** - can't see the message flow
4. **Focus on protocol details** - missed high-level architecture flaw

**The Core Issue:**
- SSE should be **event-driven** (triggered by requests)
- Currently SSE is **polling-driven** (constantly reading)
- MCP servers only respond **after receiving requests**
- Without requests, ReadMessage blocks indefinitely

### 🎯 **FINAL SOLUTION NEEDED**

**Required Changes:**
1. **Remove continuous ReadMessage loop** from SSE handler
2. **Add request/response coordination** between session endpoint and SSE
3. **Make SSE truly asynchronous** - only send when there's data

**Evidence This Will Work:**
- ✅ Claude.ai successfully connects to SSE
- ✅ Session endpoints work correctly  
- ✅ MCP servers respond to direct requests
- ❌ **Just need to connect the pieces properly**

### 🎯 **BREAKTHROUGH #4: CONCURRENCY RACE CONDITION IDENTIFIED**

**Latest Evidence from Architectural Fix Test:**
```
06:24:51 DEBUG: Read message from server memory: {"result":{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"memory-server","version":"0.6.3"}},"jsonrpc":"2.0","id":"init-arch-fix"}
06:25:21 WARNING: ReadMessage timeout/cancellation for server memory: context deadline exceeded
06:25:21 ERROR: Failed to read initialize response from MCP server memory: context deadline exceeded
```

**Critical Discovery:**
🔍 **Memory server responds correctly** - valid initialize response logged
❌ **Concurrent ReadMessage calls interfering** - race condition between code paths
⏱️ **30-second gap** - message read at 06:24:51, timeout at 06:25:21

### 💡 **THE CONCURRENCY PROBLEM**

**Root Cause:**
- **Multiple code paths** trying to read from same MCP server simultaneously  
- **ReadMessage race condition** - one path gets the message, other times out
- **Shared stdout stream** - MCP servers only have one stdout pipe

**Likely Culprits:**
1. Old synchronous handshake code still active
2. Session endpoint ReadMessage calls  
3. Possible duplicate message reading

**Solution Required:**
- **Single unified code path** for all MCP server communication
- **Remove duplicate ReadMessage calls**
- **Ensure only one reader per MCP server**

### 🏁 **FINAL SOLUTION PATH**

**Status**: 99% complete - protocol works, just need to fix concurrency
1. ✅ SSE connections work perfectly
2. ✅ Server selection fixed (notionApi removed)  
3. ✅ Architecture fixed (no more polling)
4. ✅ Memory server responds correctly
5. 🔧 **Fix ReadMessage concurrency** - FINAL STEP

### 💡 **BREAKTHROUGH #5: CONCURRENCY RACE CONDITION RESOLVED**

**Latest Progress - June 24, 06:45 UTC:**

#### **Root Cause Identified and Fixed:**
**Problem**: Two handlers were processing the same requests, causing ReadMessage race conditions:
1. `handleMCPMessage` (for `/{server}/sse` POST requests) 
2. `handleSessionMessage` (for `/{server}/sessions/{sessionId}` requests)

**Evidence**: Both handlers called ReadMessage on same MCP server simultaneously:
- `handleMCPMessage` → `handleHandshakeMessage` → `handleInitialize` → `mcpServer.ReadMessage()` 
- `handleSessionMessage` → `mcpServer.ReadMessage()`

**Solution Applied**: Added routing guard in `handleMCPMessage` to prevent processing session endpoint requests:
```go
// CRITICAL FIX: Only handle handshake messages if this is NOT a session endpoint request
isSessionEndpointRequest := strings.Contains(r.URL.Path, "/sessions/")
if isSessionEndpointRequest {
    log.Printf("ERROR: Session endpoint request incorrectly routed to handleMCPMessage")
    http.Error(w, "Internal routing error", http.StatusInternalServerError)
    return
}
```

#### **Current Status:**
- ✅ **Concurrency race condition fixed** in code
- ✅ **Container restarted** with updated fix
- 🔧 **Domain connectivity issue** preventing final testing

#### **Next Steps:**
1. **Resolve domain routing** - external domain `mcp.home.pezzos.com` returning 404
2. **Test complete fix** once connectivity restored
3. **Verify Claude.ai integration** works end-to-end

#### **Expected Outcome:**
With the race condition fixed, Claude.ai should successfully:
1. Establish SSE connection
2. Send initialize request to session endpoint
3. Receive initialize response synchronously  
4. Send tools/list request
5. Receive tools list and enable tool usage

### 🧠 **LESSONS LEARNED**

#### **Investigation Mistakes:**
1. **Assumed wrong server**: Tested memory directly vs Claude.ai's actual choice
2. **Focused on protocol**: When the issue was server-level failure
3. **Debug tunnel vision**: Spent time on session logic when server selection was the issue

#### **System Behavior:**
1. **Claude.ai server selection**: Uses `/listmcp` and picks servers (possibly first one?)
2. **Single server failure**: Breaks entire integration, no fallback
3. **SSE with broken servers**: Causes infinite timeout loops

#### **Future Improvements Needed:**
1. **Server health checks**: Don't list unhealthy servers in `/listmcp`
2. **Graceful degradation**: Handle broken servers without breaking SSE
3. **Better error reporting**: Distinguish server-level vs protocol-level failures
4. **Test methodology**: Always test the path Claude.ai actually takes

## Command Translation Analysis - June 24, 06:50 UTC

### 🔍 **CLAUDE.AI EXPECTED COMMANDS vs LOCAL MCP COMMANDS**

#### **Claude.ai Remote MCP Integration Expectations**
Based on Cloudflare MCP server analysis, Claude.ai expects these high-level capabilities:

**Core Protocol Methods:**
- `initialize` - Protocol handshake with capabilities negotiation
- `notifications/initialized` - Handshake completion
- `tools/list` - Discover available tools 
- `tools/call` - Execute specific tools
- `resources/list` - List available resources (optional)
- `resources/read` - Read specific resources (optional)
- `prompts/list` - List available prompts (optional)
- `prompts/get` - Get specific prompts (optional)

**Expected Tool Types (from Cloudflare examples):**
- **Granular, action-specific tools** (e.g., `kv_namespace_create`, `worker_deploy`)
- **Snake_case naming convention** for tool names
- **Detailed descriptions** for AI context understanding
- **Zod schema-validated parameters** with clear input/output specs
- **Specialized server domains** (e.g., Workers, Analytics, Radar, etc.)

#### **Local MCP Server Capabilities**

**Memory Server (@modelcontextprotocol/server-memory) - 9 Tools:**
1. `create_entities` - Create multiple new entities in knowledge graph
2. `create_relations` - Record how entities relate to each other  
3. `add_observations` - Record facts about existing entities
4. `delete_entities` - Remove entities from knowledge graph
5. `delete_observations` - Remove specific observations from entities
6. `delete_relations` - Remove relationships between entities
7. `read_graph` - Retrieve entire knowledge graph
8. `search_nodes` - Find relevant information by searching nodes
9. `open_nodes` - Retrieve specific entities from knowledge graph

**Sequential Thinking Server (@modelcontextprotocol/server-sequential-thinking) - 1 Tool:**
1. `sequential_thinking` - Dynamic problem-solving through structured thinking process

#### **Proxy Translation Layer Analysis**

**✅ TRANSLATION STRENGTHS:**

1. **Tool Name Normalization** (`protocol/translator.go:459-500`):
   - Converts hyphenated names to snake_case: `API-get-user` → `api_get_user`
   - Handles Claude.ai naming expectations correctly
   - Preserves original tool functionality

2. **Tool Name Denormalization** (`protocol/translator.go:502-528`):
   - Converts back for MCP server calls: `api_get_user` → `API-get-user`
   - Ensures local MCP servers receive expected format

3. **Protocol Format Translation**:
   - **Remote MCP** ↔ **JSON-RPC 2.0** conversion working correctly
   - Message type detection (`request` vs `response`) implemented
   - Error handling with proper JSON-RPC error codes

4. **Capabilities Advertisement** (`protocol/translator.go:288-299`):
   ```go
   state.Capabilities = map[string]interface{}{
       "tools": map[string]interface{}{
           "listChanged": true, // Enables Claude.ai tool discovery
       },
       "resources": map[string]interface{}{
           "listChanged": true,
       },
       "prompts": map[string]interface{}{
           "listChanged": true,
       },
   }
   ```

5. **Fallback Response Handling** (`protocol/translator.go:375-403`):
   - Provides empty lists for unsupported methods (`resources/list`, `prompts/list`)
   - Prevents Claude.ai from failing on optional capabilities

**✅ PROTOCOL COMPLIANCE:**

1. **MCP Protocol Version**: Uses `2024-11-05` (current standard)
2. **JSON-RPC 2.0**: Proper format validation and error codes
3. **Remote MCP Format**: Correct message structure with `type`, `method`, `params`
4. **Session Management**: Proper session lifecycle with initialization states

#### **🎯 TRANSLATION COMPATIBILITY ASSESSMENT**

**FULLY COMPATIBLE AREAS:**
- ✅ **Tool Discovery Flow**: `tools/list` → normalization → Claude.ai reception
- ✅ **Tool Execution Flow**: Claude.ai call → denormalization → MCP server execution  
- ✅ **Protocol Handshake**: `initialize` / `initialized` sequence works correctly
- ✅ **Capability Negotiation**: Proper capability flags for tool discovery
- ✅ **Error Handling**: JSON-RPC error responses properly formatted

**DESIGN ALIGNMENT:**
- ✅ **Memory Server Tools**: Perfect match for Claude.ai's knowledge management needs
- ✅ **Sequential Thinking**: Aligns with Claude.ai's reasoning capabilities
- ✅ **Tool Granularity**: Both servers provide focused, specific tools as expected
- ✅ **Naming Convention**: Proxy handles conversion between formats seamlessly

#### **🔧 IDENTIFIED GAPS (Non-Critical)**

1. **Resource/Prompt Support**: Local MCP servers don't expose resources/prompts, but proxy provides empty fallbacks (✅ handled)

2. **Advanced Tool Features**: 
   - Local servers use simpler parameter schemas vs Zod validation
   - **Impact**: None - Claude.ai works with any valid JSON schema

3. **Server Specialization**:
   - Cloudflare uses domain-specific servers (Workers, Analytics, etc.)
   - Our setup uses general-purpose servers (Memory, Sequential Thinking)
   - **Impact**: None - tool functionality is what matters

#### **🎉 CONCLUSION: TRANSLATION LAYER IS CORRECT**

**KEY FINDING**: The proxy translation between local MCP and Claude.ai expectations is **fully functional and compliant**. The issue is NOT in the translation layer.

**Evidence:**
1. ✅ Tool name normalization/denormalization works correctly
2. ✅ Protocol format translation (Remote MCP ↔ JSON-RPC) implemented properly  
3. ✅ Capabilities advertisement matches Claude.ai requirements
4. ✅ Message flow and session handling follows MCP specification
5. ✅ Local MCP servers provide appropriate tools for Claude.ai usage

**The translation layer successfully bridges the gap between:**
- **Claude.ai expectations**: Remote MCP protocol, snake_case tools, capabilities negotiation
- **Local MCP reality**: JSON-RPC protocol, various naming conventions, tool-focused servers

~~**Root cause of current issues lies in the concurrent request handling and SSE event loop, NOT in command translation.**~~

## 🚨 **BREAKTHROUGH #6: URL FORMAT ISSUE DISCOVERED - JUNE 24, 07:00 UTC**

### **CRITICAL DISCOVERY: Claude.ai Expects Standard Remote MCP URL Format**

**The ACTUAL Root Problem**: Our URL format doesn't match Remote MCP standard!

#### **Current Implementation (WRONG):**
```
https://mcp.domain.com/memory/sse              ← Path-based routing
https://mcp.domain.com/memory/sessions/123     ← Multiple path segments  
```

#### **Standard Remote MCP Pattern (CORRECT):**
```
https://example.com/sse                        ← Root-level SSE endpoint
https://example.com/messages                   ← Root-level messages
```

**Evidence from Working Examples:**
- ✅ `remote-mcp-server-authless.workers.dev/sse`
- ✅ `http://localhost:8080/sse` 
- ✅ `http://example.com/sse`

**ALL use ROOT-LEVEL `/sse` endpoints, NOT path-based routing!**

#### **Why Claude.ai Fails:**
1. **Claude.ai expects**: `https://mcp.domain.com/sse`
2. **We provide**: `https://mcp.domain.com/memory/sse`  
3. **Claude.ai gets confused** by path segments before `/sse`
4. **Connection fails** because URL format doesn't match Remote MCP spec

#### **SOLUTION OPTIONS:**

**Option 1: Subdomain-based (RECOMMENDED)**
```
https://memory.mcp.domain.com/sse
https://sequential-thinking.mcp.domain.com/sse
```

**Option 2: Root-level with query params**
```
https://mcp.domain.com/sse?server=memory
https://mcp.domain.com/sessions/123?server=memory
```

**Option 3: Single server per domain (simplest)**
```
https://mcp.domain.com/sse  (only serves one MCP server)
```

#### **IMPACT ASSESSMENT:**
- 🎯 **This explains EVERYTHING**: SSE hangs, session issues, tool discovery failures
- 🔧 **Simple fix**: Change URL routing to match Remote MCP standard  
- ✅ **Protocol implementation is correct**: Just wrong URL format
- 🚀 **High confidence this will work**: Matches all working examples

#### **NEXT STEPS:**
1. **IMMEDIATE**: Implement subdomain-based routing
2. **Test**: Connect Claude.ai to `https://memory.mcp.domain.com/sse`
3. **Verify**: Tool discovery and execution work correctly

## Success Criteria

### Minimum Viable Fix
- [x] SSE connection establishes without hanging
- [x] Session endpoint receives requests
- [ ] Initialize request completes successfully ← **CURRENT FOCUS**
- [ ] Claude.ai connects and discovers tools from memory server  
- [ ] tools/list returns normalized tool definitions

### Complete Solution
- [ ] All MCP servers work reliably
- [ ] Multiple concurrent Claude.ai connections supported
- [ ] Proper error handling and recovery
- [ ] Production-ready logging and monitoring