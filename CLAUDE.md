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
1. **TodoWrite Usage Protocol**:
   - **REQUIRED for**: Multi-step tasks (3+ steps), architectural changes, debugging complex issues, feature implementations, integration work
   - **OPTIONAL for**: Single-step tasks, simple file edits, basic queries, straightforward fixes
   - **Examples requiring TodoWrite**: Adding Traefik integration, implementing new MCP servers, fixing multi-component bugs, deployment workflows
   - **Examples not requiring TodoWrite**: Fixing a typo, adding a comment, reading a single file, answering questions
   - Always mark todos as completed immediately after finishing each task
2. **Run linting commands** after code changes: `go fmt ./...` and `go vet ./...`
3. **Verify builds** with `go build -o remote-mcp-proxy .`
4. **Update documentation** in `docs/` when making architectural changes
5. **Follow error handling patterns** - log both success and failure with appropriate levels

### Tool Usage Guidelines
6. **Concurrent Tool Usage**:
   - **Use concurrent calls** for: Independent file reads, parallel searches, multiple bash commands that don't depend on each other
   - **Use sequential calls** for: Dependent operations, file edits that build on previous results, error handling flows
   - **Examples of good concurrency**: Reading multiple files simultaneously, running `git status` and `git diff` in parallel, searching different patterns
   - **Examples requiring sequence**: Edit file then verify changes, read file then modify based on contents, debug then fix then test
7. **Search Tool Selection**:
   - **Use Task tool** for: Open-ended searches, keyword hunting across unknown codebases, "find me all X" queries
   - **Use Glob/Grep directly** for: Specific file patterns, known directory searches, targeted content searches
   - **Use Read directly** for: Known file paths, specific files mentioned by user
8. **Docker Tool Integration**:
   - **Available Docker MCP tools**: `mcp__docker-mcp__list-containers`, `mcp__docker-mcp__get-logs`, `mcp__docker-mcp__create-container`, `mcp__docker-mcp__deploy-compose`
   - **Use Docker MCP tools** for: Container status checks, log retrieval, deployment management
   - **Use Bash docker commands** for: Interactive operations, complex docker-compose workflows, debugging

### Deployment Protocol
9. **Docker Compose Generation**: The docker-compose.yml file is dynamically generated from template using `make`
   - Template file: `docker-compose.yml.template`
   - Generated file: `docker-compose.yml` (auto-created, not tracked in git)
   - Generation triggers: Changes to `config.json` or `.env` files
10. **Environment Configuration**: Copy and configure `.env` file before deployment:
    ```bash
    cp .env.example .env
    # Required: DOMAIN=yourdomain.com
    # Optional: ENABLE_LOCAL_TRAEFIK=true/false, ACME_EMAIL=admin@yourdomain.com
    ```
11. **Deployment Commands**: 
    - **Build and deploy**: `make up` (automatically generates docker-compose.yml from config.json and .env)
    - **Check status**: `docker-compose ps` or `make logs`
    - **Clean up**: `make down` and `make clean`
    - **Regenerate only**: `make generate`
12. **Traefik Integration Modes**:
    - **Local Traefik** (`ENABLE_LOCAL_TRAEFIK=true`): Includes Traefik service in docker-compose, manages SSL certificates, exposes ports 80/443/8080
    - **External Traefik** (`ENABLE_LOCAL_TRAEFIK=false` or omitted): Uses external 'proxy' network, requires existing Traefik setup
13. **Container Health Verification**: Always wait for container to show `(healthy)` status before testing:
    ```bash
    # Wait for healthy status
    docker-compose ps
    # Verify health endpoint
    docker exec remote-mcp-proxy curl -s http://localhost:8080/health
    ```

### Error Handling and Debugging Protocol
14. **Systematic Error Resolution**:
    - **Build Failures**: Run `go fmt ./...` and `go vet ./...`, then `go build -o remote-mcp-proxy .`
    - **Container Issues**: Check `docker logs remote-mcp-proxy`, verify health endpoint, inspect network connectivity
    - **Traefik Issues**: Verify `.env` configuration, check certificate status, inspect Traefik dashboard
    - **MCP Server Issues**: Check individual server logs, verify config.json format, test server commands manually
15. **When Lint/Typecheck Fails**:
    - **Fix Go formatting**: `go fmt ./...` (auto-fixes formatting issues)
    - **Address vet warnings**: Review `go vet ./...` output and fix reported issues
    - **Build verification**: Ensure `go build -o remote-mcp-proxy .` succeeds before deployment
    - **Dependencies**: Run `go mod tidy` if module issues occur

### Testing and Integration Rules
16. **Claude.ai Integration Testing**: If you need the user to test Claude.ai integration, just ask them directly
17. **Use Real URLs for Testing**: Always use the real domain URLs (e.g., `https://memory.mcp.home.pezzos.com/sse`) instead of localhost when testing the complete flow through Traefik
18. **Container Startup Timing**: Remember that the container takes time for its first healthcheck to pass - before the healthcheck succeeds, Traefik won't expose the service. Wait for healthy status before testing external URLs
19. **Multiple Integration Support**: ✅ **FIXED** - The proxy now supports multiple simultaneous Claude.ai integrations without tool interference. Each integration maintains isolated tool discovery and execution.
20. **Connection Cleanup**: The proxy automatically detects client disconnections within 30 seconds using keep-alive messages, with fallback cleanup after 2 minutes for stale connections.

### Connection Management
21. **Manual Cleanup**: Use `docker exec remote-mcp-proxy curl -X POST http://localhost:8080/cleanup` to force immediate cleanup of stale connections if needed
22. **Connection Monitoring**: Check active connections with `docker logs remote-mcp-proxy` - no more continuous "SSE connection active" spam messages
23. **Per-Server Request Queuing**: Requests to the same MCP server are serialized to prevent response mismatching, while different servers process requests concurrently

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