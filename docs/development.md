# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

The project lets anyone connect a local MCP server to Claude.ai by acting as a Remote MCP bridge. Even early experimental servers can be used from the web UI or smartphone app.

## Project Overview

This is a Remote MCP Proxy service that runs in Docker to bridge local MCP servers with Claude's Remote MCP protocol. The proxy serves multiple MCP servers through different URL paths (e.g., `mydomain.com/notion-mcp/sse`, `mydomain.com/memory-mcp/sse`).

## Architecture

- **Proxy Server**: HTTP server that handles incoming Remote MCP requests
- **MCP Manager**: Manages local MCP server processes and their lifecycle
- **Path Router**: Routes requests to appropriate MCP servers based on URL path
- **Config Loader**: Loads MCP server configurations from mounted config file
- **SSE Handler**: Handles Server-Sent Events for Remote MCP protocol

## Development Commands

- **Local Build**: `go build -o remote-mcp-proxy .`
- **Local Run**: `./remote-mcp-proxy` (requires config.json at /app/config.json)
- **Install Dependencies**: `go mod tidy`
- **Docker Build**: `docker build -t remote-mcp-proxy .`
- **Docker Run**: `docker run -v $(pwd)/config.json:/app/config.json -p 8080:8080 remote-mcp-proxy`
- **Docker Compose**: `make up` (recommended - generates from template)
- **Test**: `go test ./...` or `./test/run-tests.sh`
- **Test Coverage**: `go test -cover ./...`
- **Benchmarks**: `go test -bench=. ./...`
- **Lint**: `go fmt ./...` and `go vet ./...`

## Important Development Notes

**File Synchronization**: The local development files in this repository are separate from the deployed Docker server. When code changes are made locally, they must be manually synchronized to the production server before testing on the live domain. Local changes cannot be tested live until this synchronization occurs.

## Configuration

The service expects a configuration file mounted at `/app/config.json` with the same format as `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "server-name": {
      "command": "command-to-run",
      "args": ["arg1", "arg2"],
      "env": {}
    }
  }
}
```

## Traefik Integration

The proxy supports both standalone Traefik deployment and integration with existing Traefik setups through environment variable configuration.

### Quick Deployment Options

1. **With Local Traefik** (includes Traefik service):
   ```bash
   # Configure environment
   cp .env.example .env
   # Edit .env: set ENABLE_LOCAL_TRAEFIK=true and configure DOMAIN, ACME_EMAIL
   
   # Generate and deploy
   make up
   ```

2. **With Existing Traefik** (default):
   ```bash
   # Configure environment  
   cp .env.example .env
   # Edit .env: set ENABLE_LOCAL_TRAEFIK=false (or omit), configure DOMAIN
   
   # Ensure external 'proxy' network exists
   docker network create proxy
   
   # Generate and deploy
   make up
   ```

### Environment Configuration

Set these variables in `.env`:

- `ENABLE_LOCAL_TRAEFIK=true` - Include Traefik service in docker-compose
- `ENABLE_LOCAL_TRAEFIK=false` - Use external Traefik network named 'proxy'
- `DOMAIN=yourdomain.com` - Your base domain
- `ACME_EMAIL=admin@yourdomain.com` - Email for Let's Encrypt (local Traefik only)

### Key Features

- **Dynamic MCP Routing**: Automatically routes `*.mcp.yourdomain.com` to appropriate MCP servers
- **SSL/TLS Support**: Automatic Let's Encrypt certificates via HTTP challenge
- **Security**: CORS headers, security headers, and rate limiting middleware
- **Health Monitoring**: Integrated health checks and status endpoints
- **Production Ready**: Secure defaults and hardened container configuration

### Configuration Files

The `traefik/` directory contains sample configurations for advanced setups:
- `traefik.yml` - Traefik static configuration with DNS challenge support
- `dynamic.yml` - Dynamic configuration with middleware and security headers
- `INTEGRATION.md` - Detailed integration guide for complex scenarios

For complete deployment instructions, see [traefik/README.md](../traefik/README.md).

## Key Implementation Notes

- Each MCP server runs as a separate process managed by the proxy
- The proxy translates HTTP requests to MCP protocol and back
- URL paths determine which MCP server handles the request
- Server-Sent Events (SSE) are used for real-time communication as per Remote MCP spec
- Process lifecycle management ensures MCP servers are properly started/stopped
- Error handling includes both HTTP-level errors and MCP protocol errors

## Investigation Methodology

### Complex Problem-Solving Protocol
When encountering complex technical issues that require multi-step analysis:

1. **Create Investigation Structure**:
   - Use TodoWrite to break down the problem into specific, actionable steps
   - Create or update INVESTIGATIONS.md to document findings
   - Establish clear success criteria and investigation phases

2. **Systematic Root Cause Analysis**:
   - Document all symptoms and evidence observed
   - Test hypotheses systematically using available tools
   - Record breakthroughs and dead ends in INVESTIGATIONS.md
   - Update todo status in real-time as investigation progresses

3. **Evidence-Based Conclusions**:
   - Support all conclusions with specific evidence
   - Cross-reference findings across multiple sources
   - Validate solutions through testing before marking todos complete

### Investigation Documentation Format
Use this structure in INVESTIGATIONS.md:
- **Problem Statement**: Clear description of the issue
- **Evidence**: Observable symptoms and test results
- **Hypotheses**: Potential root causes with priority levels
- **Breakthroughs**: Key discoveries that change understanding
- **Solution**: Final resolution with validation evidence

## Documentation Management Protocol

### Multi-Document Synchronization
When making changes that affect multiple documentation files, update ALL relevant documents:

1. **Core Documentation Files**:
   - `README.md`: User-facing setup and usage instructions
   - `PRD.md`: Technical implementation phases and architecture
   - `CHANGELOG.md`: Version history and change tracking
   - `INVESTIGATIONS.md`: Problem analysis and solutions

2. **Update Triggers**:
   Automatically update documentation when:
   - Completing implementation phases from PRD.md
   - Adding new features or major functionality changes
   - Changing URL formats, configuration, or deployment processes
   - Fixing critical bugs or security issues
   - Making breaking changes to APIs or protocols

3. **Cross-Reference Requirements**:
   - Link related sections across documents
   - Ensure version compatibility information is consistent
   - Update all examples when changing URL formats or configuration
   - Maintain consistency in terminology and naming conventions

4. **Documentation Update Sequence**:
   - Update technical details in PRD.md first
   - Sync user-facing changes to README.md
   - Document changes in CHANGELOG.md with proper versioning
   - Reference solutions in INVESTIGATIONS.md when applicable

## Documentation Management

### PRD Progress Tracking Protocol
**MANDATORY**: When updating PRD.md implementation status, follow this exact sequence:

1. **Implementation Verification First**:
   - Complete the Implementation Verification Protocol (above)
   - Verify the feature actually works as specified
   - Confirm all related components are implemented

2. **PRD Status Update Rules**:
   - Only change status from "PLANNED" to "COMPLETED" after verification
   - Use intermediate status "IN PROGRESS" while working
   - Add completion dates in format: `✅ **COMPLETED** (YYYY-MM-DD)`
   - Include brief implementation notes if complex

3. **Cross-Reference Validation**:
   - Ensure PRD status matches actual code implementation
   - Verify all sub-steps within a phase are complete
   - Check that dependencies between phases are satisfied

4. **Documentation Synchronization**:
   - Update README.md if user-facing features changed
   - Update CHANGELOG.md with the completion
   - Ensure all examples and URLs reflect new implementation

**CRITICAL**: Never update PRD.md status without completing implementation verification first.

### Automatic Updates
When making significant changes to the codebase, automatically update the following files:

1. **README.md**: Update to reflect current implementation status, features, and architecture
2. **PRD.md**: Mark phases as completed when implementation is finished, update progress tracking
3. **CHANGELOG.md**: Document all changes following semantic versioning and Keep a Changelog format

### Update Triggers
Automatically update documentation when:
- Completing implementation phases from PRD.md
- Adding new features or major functionality
- Changing architecture or core components
- Modifying configuration format or deployment process
- Adding new development commands or processes
- Fixing bugs or security issues
- Making breaking changes

### CHANGELOG.md Management Protocol

**MANDATORY**: For every feature completion or significant change, follow this exact process:

#### 1. Update CHANGELOG.md Entry Format
```markdown
## [Unreleased]

### Added
- **Feature Name**: Brief description of what was added
- **Enhancement**: Description of improvement with context

### Fixed  
- **Bug Description**: What was broken and how it was fixed
- **Issue Reference**: Link to issue if applicable

### Changed
- **Component**: What was modified and why
- **Breaking Change**: Mark breaking changes clearly

### Security
- **Security Fix**: Description of security improvement
```

#### 2. Commit Preparation Process
**ALWAYS follow this sequence:**

1. **Update Documentation**:
   - Update README.md if features/architecture changed
   - Update PRD.md progress tracking if phases completed
   - Update CHANGELOG.md with all changes made

2. **Pre-Commit Checks**:
   ```bash
   # MANDATORY: Format and validate code
   go fmt ./...
   go vet ./...
   
   # CRITICAL: Build verification (must succeed)
   go build -o remote-mcp-proxy .
   
   # REQUIRED: Test execution (must pass)
   ./test/run-tests.sh
   
   # VERIFY: Binary creation and functionality
   ./remote-mcp-proxy --help || echo "Build verification complete"
   ```

3. **Prepare Commit**:
   - Stage all changes including documentation updates
   - Use conventional commit format: `type(scope): description`
   - Include CHANGELOG.md in the same commit as the feature

#### 3. Commit Message Convention
Use this exact format:
```
type(scope): brief description

- Updated CHANGELOG.md with [feature/fix/change] details
- [Additional context if needed]

🤖 Generated with Claude Code
```

**Commit Types:**
- `feat`: New feature
- `fix`: Bug fix  
- `docs`: Documentation only
- `refactor`: Code refactoring
- `perf`: Performance improvement
- `test`: Adding/updating tests
- `chore`: Maintenance tasks

#### 4. Automated Commit Creation
**When ready to commit, always run this sequence:**

```bash
# Add all changes including documentation
git add .

# Create commit with proper message
git commit -m "$(cat <<'EOF'
type(scope): [FEATURE/FIX DESCRIPTION]

- Updated CHANGELOG.md with [change type] details
- [Brief description of main changes]
- [Any breaking changes or migration notes]

🤖 Generated with Claude Code

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"

# Verify commit is ready (but don't push - user handles that)
git status
```

#### 5. Version Tagging Guidelines
When a release is ready:
- Move entries from `[Unreleased]` to `[version] - date`
- Follow semantic versioning (MAJOR.MINOR.PATCH)
- Create git tag: `git tag v1.2.0`

#### 6. Breaking Change Documentation
For breaking changes, always include:
- **Migration Guide**: Step-by-step upgrade instructions
- **Deprecation Notice**: What's being removed and timeline
- **Alternative Approach**: Recommended new method

**Example Breaking Change Entry:**
```markdown
### Changed
- **BREAKING**: Session management now requires `Mcp-Session-Id` header
  - **Migration**: Update client code to include session header
  - **Timeline**: Legacy support removed in v2.0.0
  - **Alternative**: Use new session-based authentication flow
```

## Error Handling and Logging Requirements

### Mandatory Error Handling
**REQUIREMENT**: Surround all actions with `try`/`except` blocks where failures are possible and log both success and failure using appropriate log levels.

### Logging Levels
Use these specific log levels consistently:

- **ERROR**: Failed operations, exceptions, critical errors that affect functionality
- **WARNING**: Potential issues, recoverable errors, deprecated usage
- **INFO**: Important operational events, startup/shutdown, configuration loading
- **DEBUG**: Detailed diagnostic information, request/response details, internal state

### Error Handling Patterns

```go
// Example for Go error handling with logging
func (s *Server) handleOperation() error {
    log.Printf("INFO: Starting operation")
    
    if err := s.performAction(); err != nil {
        log.Printf("ERROR: Operation failed: %v", err)
        return fmt.Errorf("failed to perform action: %w", err)
    }
    
    log.Printf("INFO: Operation completed successfully")
    return nil
}

// Example for critical operations
func (m *Manager) startServer(name string) {
    log.Printf("INFO: Starting MCP server: %s", name)
    
    if err := m.launchProcess(name); err != nil {
        log.Printf("ERROR: Failed to start server %s: %v", name, err)
        // Handle graceful degradation
        return
    }
    
    log.Printf("INFO: Successfully started MCP server: %s", name)
}
```

### Required Error Handling Areas
Apply error handling and logging to:

- Network operations (HTTP requests, SSE connections)
- Process management (starting/stopping MCP servers)
- File operations (configuration loading, reading/writing)
- Protocol translation and message parsing
- Authentication and authorization checks
- Database operations (if added)
- External service calls (if added)

### Success/Failure Logging
Always log both outcomes:
- **Success**: Log successful completion with INFO level
- **Failure**: Log errors with ERROR level, include context and error details
- **Warnings**: Log potential issues or recoverable errors with WARNING level
- **Debug**: Log detailed diagnostic information with DEBUG level

## Development Efficiency Rules

### Implementation Verification Protocol
**MANDATORY**: Before marking any implementation as "COMPLETED", follow this verification sequence:

1. **File Content Verification**:
   - Use Read tool to examine actual file contents
   - Verify the claimed implementation actually exists in the code
   - Check that the implementation matches the specification
   - Confirm all required components are present

2. **Build Verification**:
   - Run `go build -o remote-mcp-proxy .` to verify compilation
   - Ensure no build errors or warnings exist
   - Test that the binary can be created successfully

3. **Functional Verification**:
   - Run relevant tests with `go test ./...` or specific test files
   - Verify that new functionality works as expected
   - Check that existing functionality still works (no regressions)

4. **Documentation Verification**:
   - Confirm all documentation updates are accurate
   - Verify examples and code snippets are correct
   - Check that status updates in PRD.md reflect actual implementation

**CRITICAL**: Never mark a task as completed without completing ALL verification steps above.

### Todo List Management
**MANDATORY**: Always use TodoWrite and TodoRead tools for complex tasks:

1. **Create todos** when tasks have 3+ steps or are non-trivial
2. **Update status** in real-time: pending → in_progress → completed
3. **Only one task** should be in_progress at a time
4. **Mark completed immediately** after finishing each task - BUT ONLY after verification
5. **Break down large features** into specific, actionable items

### Task Completion Validation Checklist
**BEFORE marking any todo as completed**, verify:

- [ ] **Implementation exists**: Actual code/config changes are present in files
- [ ] **Builds successfully**: `go build` completes without errors
- [ ] **Tests pass**: Relevant tests execute successfully
- [ ] **Documentation updated**: All related docs reflect the changes
- [ ] **PRD status accurate**: Progress tracking matches actual implementation
- [ ] **Functionality verified**: The feature works as specified

### Go Development Patterns

#### Error Handling (Go-specific)
```go
// REQUIRED: Wrap errors with context
if err != nil {
    log.Printf("ERROR: Failed to %s: %v", operation, err)
    return fmt.Errorf("failed to %s: %w", operation, err)
}

// SUCCESS logging for critical operations
log.Printf("INFO: Successfully %s", operation)
```

#### Concurrency Safety
- **Always use mutexes** for shared state (connections, server lists)
- **Use sync.RWMutex** for read-heavy operations
- **Context cancellation** for goroutine cleanup
- **Channel patterns** for process communication

#### MCP Protocol Implementation
```go
// Session management pattern
func (s *Server) handleMCPRequest(w http.ResponseWriter, r *http.Request) {
    sessionID := s.getSessionID(r)
    
    // Authentication check
    if !s.validateAuthentication(r) {
        log.Printf("ERROR: Authentication failed for session %s", sessionID)
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }
    
    log.Printf("INFO: Processing MCP request for session %s", sessionID)
    // ... handle request
}
```

## Testing Requirements

### Comprehensive Testing Strategy
- **Unit Tests**: Test individual components and functions
- **Integration Tests**: Test component interactions and workflows
- **Configuration Tests**: Test environment variables and configuration loading
- **Protocol Tests**: Test Remote MCP protocol compliance and URL routing
- **Regression Tests**: Ensure changes don't break existing functionality

### Required Test Types by Feature:
- **URL Routing Changes**: Test subdomain extraction, validation, and routing
- **Environment Configuration**: Test variable loading, defaults, and precedence
- **Protocol Changes**: Test Remote MCP compliance and message formats
- **API Changes**: Test all endpoints and response formats

### Test Implementation Standards:
- Create test files alongside implementation (`component_test.go`)
- Use table-driven tests for multiple scenarios
- Include both positive and negative test cases
- Test error conditions and edge cases
- Validate all public methods and critical private methods

### Pre-deployment Testing:
```bash
# Required test sequence before deployment
go test ./...                    # All tests must pass
go test -race ./...             # Race condition detection
go test -cover ./...            # Ensure adequate coverage
go build -o remote-mcp-proxy .  # Verify compilation
```

### Build and Test Workflow

#### Required Commands Before Completion
**ALWAYS run these commands after code changes:**
```bash
# 1. Format code
go fmt ./...

# 2. Check for issues  
go vet ./...

# 3. Build to verify compilation
go build -o remote-mcp-proxy .

# 4. Run tests (comprehensive)
./test/run-tests.sh

# OR run specific test types:
go test -v ./protocol ./mcp ./proxy  # Unit tests
go test -v .                        # Integration tests
go test -cover ./...                # With coverage
```

#### Deployment Workflow
**ALWAYS follow this sequence for deployment:**
```bash
# 1. Build and deploy container
docker-compose up -d --build

# 2. Wait for healthy status (required!)
docker-compose ps
# Wait until status shows "(healthy)" not just "(health: starting)"

# 3. Verify service is responding
docker exec remote-mcp-proxy curl -s http://localhost:8080/health

# 4. Test external URL (if using Traefik)
# Only after container is healthy - Traefik won't route until then
curl -s https://memory.mcp.home.pezzos.com/health
```

#### Development Cycle
1. **Read existing code** before making changes
2. **Use TodoWrite** for planning complex tasks
3. **Implement changes** with proper error handling
4. **Test compilation** with go build
5. **Run linting** commands
6. **Update documentation** (README.md, PRD.md) when needed
7. **Mark todos complete** immediately after finishing

### File Organization and Architecture

#### Core Components (Do NOT modify structure without planning)
- `main.go`: Application entry point, server lifecycle
- `config/`: Configuration loading and validation
- `mcp/`: MCP server process management  
- `protocol/`: Message translation and handshake handling
- `proxy/`: HTTP server, routing, and SSE handling

#### When Adding New Features
1. **Determine component** where feature belongs
2. **Check existing patterns** in that component
3. **Follow established interfaces** and error handling
4. **Add logging** for all success/failure paths
5. **Update relevant documentation**

### MCP Protocol Specific Rules

#### Handshake Implementation
- **Always validate** protocol version in initialize requests
- **Track session state** for all connections
- **Handle initialize/initialized** sequence properly
- **Clean up sessions** when connections close

#### Message Translation
- **Validate JSON-RPC** format before translation
- **Handle both directions**: Remote MCP ↔ Local MCP
- **Preserve message IDs** for proper response correlation
- **Error responses** must follow JSON-RPC 2.0 spec

#### SSE Connection Management
```go
// REQUIRED pattern for SSE handlers
defer func() {
    s.translator.RemoveConnection(sessionID)
    log.Printf("INFO: SSE connection closed for session %s", sessionID)
}()
```

### Performance and Scalability

#### Process Management
- **Non-blocking reads** from MCP server stdout
- **Graceful shutdown** with context cancellation
- **Process restart** handling for failed MCP servers
- **Resource cleanup** on connection termination

#### Memory Management
- **Remove closed connections** from state tracking
- **Limit connection lifetime** if needed
- **Clean up goroutines** with proper context usage

### Security Best Practices

#### Authentication
- **Validate bearer tokens** for production use
- **Origin checking** for CORS security
- **Rate limiting** considerations for future implementation
- **Secure logging** (don't log sensitive tokens)

#### Input Validation
```go
// REQUIRED validation pattern
if err := json.Unmarshal(data, &message); err != nil {
    log.Printf("ERROR: Invalid JSON in request: %v", err)
    s.sendErrorResponse(w, nil, protocol.ParseError, "Invalid JSON", false)
    return
}
```

### Debugging and Monitoring

#### Required Logging Points
- **Server startup/shutdown** (INFO)
- **MCP server lifecycle** (INFO for start/stop, ERROR for failures)
- **Authentication attempts** (INFO for success, ERROR for failures)
- **Protocol handshake** (INFO for completion, ERROR for failures)
- **Message translation** (DEBUG for details, ERROR for failures)

#### Health Check Implementation
- **Always include** process status in health checks
- **Monitor MCP server** health individually
- **Return appropriate HTTP status** codes

#### Monitoring API Endpoints

The proxy exposes several monitoring endpoints for health tracking and resource monitoring:

**System Health Endpoint**:
```bash
GET /health
# Returns: {"status": "healthy"}
# Use: Basic service availability check
```

**Detailed Server Health**:
```bash
GET /health/servers
# Returns: Comprehensive health status for all MCP servers
# Response: {
#   "timestamp": "2025-06-26T10:30:00Z",
#   "servers": {
#     "memory": {
#       "name": "memory",
#       "status": "healthy|unhealthy|unknown",
#       "lastCheck": "2025-06-26T10:29:45Z",
#       "responseTimeMs": 120,
#       "consecutiveFails": 0,
#       "restartCount": 0,
#       "lastError": ""
#     }
#   },
#   "summary": {
#     "total": 4,
#     "healthy": 3,
#     "unhealthy": 1,
#     "unknown": 0
#   }
# }
```

**Resource Metrics**:
```bash
GET /health/resources
# Returns: Current resource usage for all MCP processes
# Response: {
#   "timestamp": "2025-06-26T10:30:00Z",
#   "processes": [
#     {
#       "pid": 123,
#       "name": "memory-server",
#       "memoryMB": 145.2,
#       "cpuPercent": 2.1,
#       "virtualMB": 512.0,
#       "residentMB": 145.2,
#       "timestamp": "2025-06-26T10:30:00Z"
#     }
#   ],
#   "summary": {
#     "processCount": 4,
#     "totalMemoryMB": 580.5,
#     "totalCPU": 8.3,
#     "averageMemoryMB": 145.1,
#     "averageCPU": 2.1
#   }
# }
```

#### Development Monitoring Usage

**During Development**:
```bash
# Check if all servers are healthy during development
curl http://localhost:8080/health/servers

# Monitor resource usage patterns
curl http://localhost:8080/health/resources

# Basic health check for CI/CD
curl http://localhost:8080/health
```

**Production Monitoring Integration**:
- Use `/health/servers` for alerting on unhealthy MCP servers
- Monitor `/health/resources` for memory leaks and CPU issues  
- Set up automated alerts for restart count increases
- Include `/health` in uptime monitoring services

#### Health Monitoring Architecture

The monitoring system includes:

1. **Health Checker** (`health/checker.go`):
   - Periodic ping-based health checks every 30 seconds
   - Automatic server restart after 3 consecutive failures
   - Restart limits (max 3 per 5-minute window)

2. **Resource Monitor** (`monitoring/resources.go`):
   - Process-level memory/CPU tracking
   - Alert thresholds (>500MB memory, >80% CPU)
   - Automatic MCP process discovery

3. **Integration Points**:
   - Health checker initialized in `main.go`
   - Monitoring endpoints exposed in `proxy/server.go`
   - Graceful shutdown cleanup included

### Code Review and Quality

#### Before Committing
1. **All todos marked complete** for the feature
2. **Error handling implemented** for all failure points
3. **Logging added** for success and failure paths
4. **Documentation updated** if architecture changed
5. **Build passes** without warnings
6. **Vet checks pass** without issues

#### Code Style
- **Descriptive function names** that explain purpose
- **Proper Go naming conventions** (exported vs unexported)
- **Interface definitions** for testability
- **Minimal external dependencies** (prefer standard library)

### Integration with Claude.ai

#### Remote MCP Compatibility
- **Follow MCP specification** exactly for handshake
- **Support SSE reconnection** with Last-Event-ID
- **Handle multiple simultaneous** connections
- **Proper CORS headers** for web client access

#### Testing with Claude.ai Integration
- **Ask user for testing**: If you need to test Claude.ai integration, ask the user directly to test the connection
- **Use real domain URLs**: Always test with the actual domain URLs (e.g., `https://memory.mcp.home.pezzos.com/sse`) instead of localhost for complete flow validation through Traefik
- **Wait for healthcheck**: Remember the container needs time for its first healthcheck to pass - Traefik won't expose the service until the container is healthy
- **Verify complete flow**: Test OAuth flow, SSE connection, tool discovery, and actual tool execution
- **Check error responses** are properly formatted according to Remote MCP specification
- **Tool Disappearing Issue**: If tools appear initially but then disappear, this indicates request timeout problems. The proxy handles this with:
  - 10-second timeout for MCP server responses
  - Fallback responses for optional methods (resources/list, prompts/list)
  - Proper error responses for unsupported methods
  - This prevents Claude.ai from canceling connections due to timeouts
