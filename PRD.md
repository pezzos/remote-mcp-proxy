# Product Requirements Document: Remote MCP Proxy

## Problem Statement

Many MCP (Model Context Protocol) servers are designed to run locally and are not yet compatible with Claude's Remote MCP protocol. This prevents users from accessing these MCP servers through Claude's web UI, limiting their functionality to desktop applications only.

## Solution Overview

Build a Docker-based proxy service that bridges local MCP servers with Claude's Remote MCP protocol, enabling any local MCP to be accessed through Claude's web interface.

## Architecture

### Core Components

1. **HTTP Proxy Server**
   - Receives Remote MCP requests from Claude.ai
   - Routes requests based on URL path patterns
   - Handles authentication and CORS if needed

2. **MCP Process Manager**
   - Spawns and manages local MCP server processes
   - Monitors process health and restarts failed servers
   - Handles graceful shutdown of all processes

3. **Protocol Translator**
   - Converts HTTP/SSE requests to MCP JSON-RPC protocol
   - Translates MCP responses back to Remote MCP format
   - Maintains session state and connection mapping

4. **Configuration Loader**
   - Reads mounted configuration file (claude_desktop_config.json format)
   - Validates MCP server configurations
   - Supports hot-reloading of configuration changes

### URL Structure

```
https://mydomain.com/{mcp-server-name}/sse
```

Examples:
- `https://mydomain.com/notion-mcp/sse`
- `https://mydomain.com/memory-mcp/sse`
- `https://mydomain.com/filesystem-mcp/sse`

### Configuration Format

Uses the same format as `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "notion-mcp": {
      "command": "npx",
      "args": ["-y", "@anthropic-ai/mcp-server-notion"],
      "env": {
        "NOTION_TOKEN": "secret_token"
      }
    },
    "memory-mcp": {
      "command": "python",
      "args": ["-m", "memory_mcp"],
      "env": {}
    }
  }
}
```

## Technical Implementation

### Phase 1: Core Proxy Service ✅ **COMPLETED**
- [x] Set up HTTP server with path-based routing (Gorilla Mux)
- [x] Implement MCP process spawning and management
- [x] Create basic protocol translation layer (JSON-RPC ↔ Remote MCP)
- [x] Add configuration file loading (claude_desktop_config.json format)

### Phase 2: Remote MCP Protocol
- [x] Implement Server-Sent Events (SSE) endpoint
- [ ] Add Remote MCP handshake and authentication
- [x] Implement bidirectional message translation
- [x] Handle connection lifecycle management

### Phase 3: Production Features ✅ **COMPLETED**
- [x] Add health checks and monitoring (/health endpoint)
- [x] Implement graceful shutdown and process cleanup
- [x] Add logging and error handling
- [x] Create Docker image and deployment configuration

### Phase 4: Advanced Features
- [ ] Configuration hot-reloading
- [ ] Process restart policies and recovery
- [ ] Metrics and observability
- [ ] Rate limiting and security features

## Technology Stack

### Chosen Implementation
- **Go 1.21**: Selected for excellent proxy performance and concurrent process management
- **Gorilla Mux**: HTTP router for path-based routing
- **Standard Library**: Process management, JSON handling, HTTP/SSE support

### Key Dependencies
- `github.com/gorilla/mux`: HTTP routing
- Go standard library: `os/exec`, `net/http`, `encoding/json`
- Alpine Linux base image for minimal Docker footprint

## Docker Configuration

### Dockerfile Structure
```dockerfile
# Multi-stage build for optimal size
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/main .
EXPOSE 8080
CMD ["./main"]
```

### Docker Compose Example
```yaml
version: '3.8'
services:
  remote-mcp-proxy:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - ./config.json:/app/config.json:ro
    environment:
      - GO_ENV=production
```

## Security Considerations

- Validate all MCP server configurations before spawning processes
- Sanitize environment variables and command arguments
- Implement proper process isolation
- Add authentication for Remote MCP endpoints if required
- Secure handling of secrets in environment variables

## Success Criteria

1. **Functional**: Any local MCP server can be accessed through Claude.ai web UI
2. **Reliable**: Proxy handles process failures and restarts gracefully
3. **Performant**: Low latency translation between protocols
4. **Secure**: Safe execution of configured MCP servers
5. **Maintainable**: Easy to deploy and configure via Docker

## Future Enhancements

- Web-based configuration UI
- MCP server marketplace integration
- Automatic MCP server discovery
- Load balancing for multiple instances
- Advanced monitoring and alerting