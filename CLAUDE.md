# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Remote MCP Proxy service that runs in Docker to bridge local MCP servers with Claude's Remote MCP protocol. The proxy serves multiple MCP servers through different URL paths (e.g., `mydomain.com/notion-mcp/sse`, `mydomain.com/memory-mcp/sse`).

## Documentation

Complete documentation is organized in the `docs/` directory:

- **[docs/development.md](docs/development.md)** - Development guide, commands, protocols, and implementation standards
- **[docs/architecture.md](docs/architecture.md)** - Technical architecture and implementation phases  
- **[docs/troubleshooting.md](docs/troubleshooting.md)** - Problem analysis and debugging guides

## Quick Reference

### Development Commands
- **Local Build**: `go build -o remote-mcp-proxy .`
- **Test**: `go test ./...` or `./test/run-tests.sh`
- **Lint**: `go fmt ./...` and `go vet ./...`
- **Docker Build**: `docker build -t remote-mcp-proxy .`

### Key Development Rules
1. **Always use TodoWrite** for planning complex tasks
2. **Run linting commands** after code changes: `go fmt ./...` and `go vet ./...`
3. **Verify builds** with `go build -o remote-mcp-proxy .`
4. **Update documentation** in `docs/` when making architectural changes
5. **Follow error handling patterns** - log both success and failure with appropriate levels

### Deployment Protocol
6. **Docker Compose Generation**: The docker-compose.yml file is dynamically generated from template using `make`
7. **Deployment Commands**: 
   - Build and deploy: `make up` (automatically generates docker-compose.yml from config.json and .env)
   - Check status: `docker-compose ps` or `make logs`
   - Clean up: `make down` and `make clean`
8. **Traefik Integration**: Set `ENABLE_LOCAL_TRAEFIK=true` in `.env` to include Traefik service, or `false` to use external Traefik
9. **Container Health Verification**: Always wait for container to show `(healthy)` status before testing:
   ```bash
   # Wait for healthy status
   docker-compose ps
   # Verify health endpoint
   docker exec remote-mcp-proxy curl -s http://localhost:8080/health
   ```

### Testing and Integration Rules
10. **Claude.ai Integration Testing**: If you need the user to test Claude.ai integration, just ask them directly
11. **Use Real URLs for Testing**: Always use the real domain URLs (e.g., `https://memory.mcp.home.pezzos.com/sse`) instead of localhost when testing the complete flow through Traefik
12. **Container Startup Timing**: Remember that the container takes time for its first healthcheck to pass - before the healthcheck succeeds, Traefik won't expose the service. Wait for healthy status before testing external URLs
13. **Multiple Integration Support**: ✅ **FIXED** - The proxy now supports multiple simultaneous Claude.ai integrations without tool interference. Each integration maintains isolated tool discovery and execution.
14. **Connection Cleanup**: The proxy automatically detects client disconnections within 30 seconds using keep-alive messages, with fallback cleanup after 2 minutes for stale connections.

### Connection Management
15. **Manual Cleanup**: Use `docker exec remote-mcp-proxy curl -X POST http://localhost:8080/cleanup` to force immediate cleanup of stale connections if needed
16. **Connection Monitoring**: Check active connections with `docker logs remote-mcp-proxy` - no more continuous "SSE connection active" spam messages
17. **Per-Server Request Queuing**: Requests to the same MCP server are serialized to prevent response mismatching, while different servers process requests concurrently

### Configuration
Service expects `/app/config.json` with same format as `claude_desktop_config.json`:
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

For complete development guidelines, implementation standards, testing requirements, and troubleshooting procedures, see the documentation in the `docs/` directory.