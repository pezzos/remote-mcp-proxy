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

### Phase 1: Core Proxy Service
- [ ] Set up HTTP server with path-based routing
- [ ] Implement MCP process spawning and management
- [ ] Create basic protocol translation layer
- [ ] Add configuration file loading

### Phase 2: Remote MCP Protocol
- [ ] Implement Server-Sent Events (SSE) endpoint
- [ ] Add Remote MCP handshake and authentication
- [ ] Implement bidirectional message translation
- [ ] Handle connection lifecycle management

### Phase 3: Production Features
- [ ] Add health checks and monitoring
- [ ] Implement graceful shutdown and process cleanup
- [ ] Add logging and error handling
- [ ] Create Docker image and deployment configuration

### Phase 4: Advanced Features
- [ ] Configuration hot-reloading
- [ ] Process restart policies and recovery
- [ ] Metrics and observability
- [ ] Rate limiting and security features

## Technology Stack

### Language Options
- **Node.js**: Good ecosystem for HTTP/SSE and process management
- **Python**: Strong MCP ecosystem, good for protocol handling
- **Go**: Excellent for proxy services and concurrent process management

### Key Dependencies
- HTTP server framework
- Process management utilities
- JSON-RPC protocol handling
- Server-Sent Events implementation
- Configuration file parsing

## Docker Configuration

### Dockerfile Structure
```dockerfile
FROM node:18-alpine  # or python:3.11-alpine
WORKDIR /app
COPY package*.json ./  # or requirements.txt
RUN npm install  # or pip install
COPY . .
EXPOSE 8080
CMD ["npm", "start"]  # or python main.py
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
      - NODE_ENV=production
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