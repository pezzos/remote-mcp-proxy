# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Remote MCP Proxy service that runs in Docker to bridge local MCP servers with Claude's Remote MCP protocol. The proxy serves multiple MCP servers through different URL paths (e.g., `mydomain.com/notion-mcp/sse`, `mydomain.com/memory-mcp/sse`).

## Architecture

- **Proxy Server**: HTTP server that handles incoming Remote MCP requests
- **MCP Manager**: Manages local MCP server processes and their lifecycle
- **Path Router**: Routes requests to appropriate MCP servers based on URL path
- **Config Loader**: Loads MCP server configurations from mounted config file
- **SSE Handler**: Handles Server-Sent Events for Remote MCP protocol

## Development Commands

Since this is a new project, common commands will be added as the build system is established:

- Build: `docker build -t remote-mcp-proxy .`
- Run: `docker run -v /path/to/config.json:/app/config.json -p 8080:8080 remote-mcp-proxy`
- Test: (to be determined based on chosen test framework)
- Lint: (to be determined based on chosen language and tooling)

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