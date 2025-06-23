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
- **Docker Compose**: `docker-compose up -d`
- **Test**: `go test ./...` or `./test/run-tests.sh`
- **Test Coverage**: `go test -cover ./...`
- **Benchmarks**: `go test -bench=. ./...`
- **Lint**: `go fmt ./...` and `go vet ./...`

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

## Key Implementation Notes

- Each MCP server runs as a separate process managed by the proxy
- The proxy translates HTTP requests to MCP protocol and back
- URL paths determine which MCP server handles the request
- Server-Sent Events (SSE) are used for real-time communication as per Remote MCP spec
- Process lifecycle management ensures MCP servers are properly started/stopped
- Error handling includes both HTTP-level errors and MCP protocol errors

## Documentation Management

### Automatic Updates
When making significant changes to the codebase, automatically update the following files:

1. **README.md**: Update to reflect current implementation status, features, and architecture
2. **PRD.md**: Mark phases as completed when implementation is finished, update progress tracking

### Update Triggers
Automatically update documentation when:
- Completing implementation phases from PRD.md
- Adding new features or major functionality
- Changing architecture or core components
- Modifying configuration format or deployment process
- Adding new development commands or processes

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

### Todo List Management
**MANDATORY**: Always use TodoWrite and TodoRead tools for complex tasks:

1. **Create todos** when tasks have 3+ steps or are non-trivial
2. **Update status** in real-time: pending → in_progress → completed
3. **Only one task** should be in_progress at a time
4. **Mark completed immediately** after finishing each task
5. **Break down large features** into specific, actionable items

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

#### Testing with Claude
- **Use MCP inspector** tool for validation
- **Test OAuth flow** if authentication enabled  
- **Verify SSE streams** work correctly
- **Check error responses** are properly formatted
