# Remote MCP Proxy

A Docker-based proxy service built in Go that enables local MCP (Model Context Protocol) servers to be accessed through Claude's web UI by bridging them with the Remote MCP protocol.

## Problem

Many MCP servers are designed to run locally and aren't compatible with Claude's Remote MCP protocol. This limits their use to desktop applications and prevents access through Claude's web interface.

## Solution

This proxy service runs in Docker and:
- Manages local MCP server processes with health monitoring
- Translates between HTTP/SSE and MCP JSON-RPC protocols
- Serves multiple MCP servers through different URL paths
- Uses the same configuration format as Claude Desktop
- Provides graceful shutdown and process cleanup
- Includes health check endpoints for monitoring

## Quick Start

### 1. Create Configuration File

Create a `config.json` file with your MCP servers (same format as `claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "notion-mcp": {
      "command": "npx",
      "args": ["-y", "@anthropic-ai/mcp-server-notion"],
      "env": {
        "NOTION_TOKEN": "your_notion_token_here"
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

### 2. Run with Docker

```bash
# Build the image
docker build -t remote-mcp-proxy .

# Run the proxy
docker run -d \
  --name mcp-proxy \
  -p 8080:8080 \
  -v $(pwd)/config.json:/app/config.json:ro \
  remote-mcp-proxy
```

### 3. Or Use Docker Compose

```bash
docker-compose up -d
```

### 4. Configure Claude.ai

In Claude's web UI, add your remote MCP servers using these URLs:
- `https://your-domain.com/notion-mcp/sse`
- `https://your-domain.com/memory-mcp/sse`

Replace `your-domain.com` with your actual domain where the proxy is hosted.

## URL Structure

Each MCP server is available at:
```
https://your-domain.com/{server-name}/sse
```

Where `{server-name}` matches the key in your `config.json` file.

## Configuration

The proxy uses the same configuration format as Claude Desktop's `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "server-name": {
      "command": "command-to-run",
      "args": ["arg1", "arg2"],
      "env": {
        "ENV_VAR": "value"
      }
    }
  }
}
```

### Environment Variables

- Set environment variables for your MCP servers in the `env` section
- Store secrets securely and reference them in your Docker deployment
- The proxy will pass these environment variables to the spawned MCP processes

## Docker Compose

For easier deployment, use Docker Compose:

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
    restart: unless-stopped
```

## Development

### Prerequisites

- Go 1.21 or later
- Docker
- Your MCP servers' dependencies (Node.js, Python, etc.)

### Local Development

```bash
# Clone the repository
git clone <repository-url>
cd remote-mcp-proxy

# Install Go dependencies
go mod tidy

# Build locally
go build -o remote-mcp-proxy .

# Run locally (requires config.json at /app/config.json)
./remote-mcp-proxy

# Or build and run with Docker
docker build -t remote-mcp-proxy .
docker run -v $(pwd)/config.json:/app/config.json -p 8080:8080 remote-mcp-proxy
```

### Development Commands

- **Build**: `go build -o remote-mcp-proxy .`
- **Run**: `./remote-mcp-proxy`
- **Test**: `go test ./...`
- **Lint**: `go fmt ./...` and `go vet ./...`
- **Dependencies**: `go mod tidy`

### Adding New MCP Servers

1. Add the server configuration to `config.json`
2. Restart the proxy container
3. The new server will be available at `/{server-name}/sse`

## Architecture

The proxy is built in Go and consists of:

- **HTTP Proxy Server**: Handles incoming Remote MCP requests using Gorilla Mux router
- **MCP Process Manager**: Spawns and manages local MCP server processes with health monitoring
- **Protocol Translator**: Converts between HTTP/SSE and MCP JSON-RPC protocols
- **Configuration Loader**: Reads and validates MCP server configs (claude_desktop_config.json format)
- **SSE Handler**: Implements Server-Sent Events for real-time Remote MCP communication

### Technology Stack

- **Go 1.21**: Core language for performance and concurrency
- **Gorilla Mux**: HTTP routing and path-based server selection
- **Standard Library**: Process management (`os/exec`), HTTP/SSE, JSON handling
- **Alpine Linux**: Minimal Docker base image for production deployment

## Monitoring and Health Checks

The proxy includes a health check endpoint:

```bash
# Check proxy health
curl http://localhost:8080/health
```

## Troubleshooting

### MCP Server Won't Start
- Check the command and arguments in your config
- Verify environment variables are set correctly
- Look at proxy logs for process spawn errors
- Ensure required dependencies are available in the container

### Connection Issues
- Ensure the proxy is accessible from Claude.ai
- Check firewall and network configuration
- Verify SSL/TLS setup for HTTPS endpoints
- Test the SSE endpoint directly: `curl http://localhost:8080/{server-name}/sse`

### Protocol Errors
- Confirm your MCP server supports the expected protocol version
- Check for proper JSON-RPC message formatting
- Review SSE connection handling
- Monitor proxy logs for translation errors

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test with multiple MCP servers
5. Submit a pull request

## License

[Add your license here]

## Related Projects

- [MCP Specification](https://spec.modelcontextprotocol.io/)
- [Claude Desktop MCP Integration](https://support.anthropic.com/en/articles/11175166-about-custom-integrations-using-remote-mcp)
- [Official MCP Servers](https://github.com/modelcontextprotocol)