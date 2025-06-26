# PLAN.md - Multi-Instance and Multi-User Support for Remote MCP Proxy

## Overview

This plan outlines the necessary tasks to enable proper support for:
1. **Phase 1**: Multiple Claude.ai instances per user (web + desktop) with isolated data
2. **Phase 2**: Multi-user support with proper tenant isolation

## Current State Analysis

### What Works Now
- ✅ Multiple Claude.ai instances can connect simultaneously
- ✅ Each connection gets a unique session ID and protocol state
- ✅ OAuth authentication flow is implemented
- ✅ Connection lifecycle management is robust

### Critical Limitations
- ❌ All instances share the same MCP server processes (shared state)
- ❌ No data isolation between instances
- ❌ Request serialization creates performance bottlenecks
- ❌ No user identification or tenant isolation

## Phase 1: Multiple Claude.ai Instances per User

### Goal
Enable a single user to use both Claude.ai web and desktop simultaneously without data conflicts or shared state issues.

### Task List

#### 1. Implement Per-Session MCP Server Instances
- [ ] **1.1** Modify `mcp/manager.go` to spawn separate MCP server processes per session
  - Create `StartServerForSession(serverName, sessionID string)` method
  - Maintain mapping of `sessionID -> serverProcess`
  - Update process lifecycle to be session-aware
  
- [ ] **1.2** Update request routing to session-specific servers
  - Modify `proxy/server.go` to route requests to correct server instance
  - Update `GetServer()` to accept sessionID parameter
  - Ensure proper cleanup when session ends

- [ ] **1.3** Implement session-based working directories
  - Create `/app/sessions/{sessionID}/` directories
  - Configure each MCP server instance with isolated paths
  - Handle cleanup of session directories on disconnect

#### 2. Session-Aware Configuration
- [ ] **2.1** Extend config.json to support per-session environment variables
  ```json
  {
    "mcpServers": {
      "memory": {
        "env": {
          "MEMORY_FILE_PATH": "/app/sessions/{SESSION_ID}/memory.json"
        }
      }
    }
  }
  ```
  
- [ ] **2.2** Implement template variable substitution
  - Replace `{SESSION_ID}` with actual session ID at runtime
  - Support other variables like `{USER_ID}`, `{INSTANCE_TYPE}`

#### 3. Resource Management
- [ ] **3.1** Implement process limits per session
  - Maximum number of MCP servers per session
  - Memory/CPU limits per session
  - Automatic cleanup of idle sessions

- [ ] **3.2** Add session monitoring
  - Track active sessions and their resource usage
  - Implement `/health/sessions` endpoint
  - Add metrics for debugging

#### 4. State Persistence (Optional)
- [ ] **4.1** Implement session reconnection
  - Allow Claude.ai to reconnect to existing session
  - Persist session state between connections
  - Configurable session lifetime

- [ ] **4.2** Add session management endpoints
  - `GET /sessions` - List active sessions
  - `DELETE /sessions/{id}` - Manual cleanup
  - `GET /sessions/{id}/state` - Debug endpoint

### Implementation Priority
1. Start with memory-only isolation (no persistence)
2. Focus on memory and filesystem MCP servers first
3. Test with 2-3 simultaneous instances before optimizing

## Phase 2: Multi-User Support

### Goal
Enable multiple users to share the same proxy URL while maintaining complete data isolation and security.

### High-Level Tasks

#### 1. User Authentication Enhancement
- [ ] **1.1** Implement proper user identification
  - Extend OAuth to include user claims
  - Map Bearer tokens to specific users
  - Consider JWT tokens for richer claims

- [ ] **1.2** User database/directory
  - Simple file-based user registry initially
  - User ID to settings mapping
  - Per-user authorization rules

#### 2. Tenant Isolation
- [ ] **2.1** User-specific namespacing
  - Modify session IDs to include user prefix
  - Separate working directories per user
  - Isolated MCP server configurations per user

- [ ] **2.2** Configuration management
  - Per-user config.json files
  - User-specific MCP server allowlists
  - Resource quotas per user

#### 3. Security Enhancements
- [ ] **3.1** Proper token validation
  - Implement token expiration
  - Secure token storage
  - Token revocation capability

- [ ] **3.2** Access control
  - Limit which MCP servers each user can access
  - Implement rate limiting per user
  - Audit logging for security

#### 4. Administration Features
- [ ] **4.1** User management API
  - Create/update/delete users
  - Set user quotas and permissions
  - Monitor user activity

- [ ] **4.2** Administrative dashboard (optional)
  - Simple web UI for user management
  - System health monitoring
  - Usage statistics

### Technical Considerations

#### Data Storage Options
1. **Simple**: File-based isolation using directories
2. **Scalable**: SQLite database per user
3. **Enterprise**: PostgreSQL with proper schemas

#### Deployment Models
1. **Personal**: Docker Compose with local volumes
2. **Team**: Kubernetes with persistent volume claims
3. **SaaS**: Cloud-native with object storage

### Migration Path
1. Start with Phase 1 (session isolation)
2. Add simple user identification
3. Implement basic tenant isolation
4. Enhance security and administration

## Testing Strategy

### Phase 1 Testing
- [ ] Test with 2 Claude.ai web tabs simultaneously
- [ ] Test web + desktop client together
- [ ] Verify memory server isolation
- [ ] Load test with 5-10 sessions
- [ ] Test session cleanup and resource management

### Phase 2 Testing
- [ ] Multi-user authentication flows
- [ ] Cross-user isolation verification
- [ ] Resource limit enforcement
- [ ] Security penetration testing
- [ ] Performance benchmarking

## Rollout Plan

### Phase 1 Timeline (Single User, Multi-Instance)
- Week 1-2: Implement per-session MCP servers
- Week 3: Session-aware configuration
- Week 4: Resource management and testing
- Week 5: Documentation and deployment

### Phase 2 Timeline (Multi-User)
- Month 2: Basic user authentication
- Month 3: Tenant isolation
- Month 4: Security and administration
- Month 5: Production hardening

## Success Metrics

### Phase 1
- ✅ User can run web + desktop Claude.ai simultaneously
- ✅ Each instance has isolated memory/state
- ✅ No performance degradation with multiple instances
- ✅ Clean session lifecycle management

### Phase 2
- ✅ Support 10+ concurrent users
- ✅ Complete data isolation between users
- ✅ < 100ms latency overhead from isolation
- ✅ Zero security vulnerabilities in isolation

## Open Questions

1. **Session Persistence**: Should sessions survive container restarts?
2. **Resource Limits**: What are reasonable limits per session/user?
3. **Billing/Quotas**: How to track usage for potential billing?
4. **Backup/Recovery**: How to handle user data backup?
5. **Compliance**: Any regulatory requirements for multi-tenant systems?

## Next Steps

1. Review and approve this plan
2. Decide on Phase 1 vs Phase 2 priority
3. Set up development environment for testing
4. Create detailed technical design for chosen phase
5. Begin implementation with highest-impact changes first