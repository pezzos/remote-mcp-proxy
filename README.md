# Remote MCP Proxy

A Docker-based proxy service that enables local MCP (Model Context Protocol) servers to be accessed through Claude's web UI by bridging them with the Remote MCP protocol.

## Problem

Many MCP servers are designed to run locally and aren't compatible with Claude's Remote MCP protocol. This limits their use to desktop applications and prevents access through Claude's web interface.

## Solution

This proxy service runs in Docker and:
- Manages local MCP server processes
- Translates between HTTP/SSE and MCP JSON-RPC protocols
- Serves multiple MCP servers through different URL paths
- Uses the same configuration format as Claude Desktop

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

### 3. Configure Claude.ai

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

- Docker
- Your MCP servers' dependencies (Node.js, Python, etc.)

### Local Development

1. Clone the repository
2. Create your `config.json` file
3. Build and run with Docker
4. Test with your MCP servers

### Adding New MCP Servers

1. Add the server configuration to `config.json`
2. Restart the proxy container
3. The new server will be available at `/{server-name}/sse`

## Architecture

- **HTTP Proxy**: Handles incoming Remote MCP requests
- **Process Manager**: Spawns and manages MCP server processes
- **Protocol Translator**: Converts between HTTP/SSE and MCP JSON-RPC
- **Configuration Loader**: Reads and validates MCP server configs

## Troubleshooting

### MCP Server Won't Start
- Check the command and arguments in your config
- Verify environment variables are set correctly
- Look at proxy logs for process spawn errors

### Connection Issues
- Ensure the proxy is accessible from Claude.ai
- Check firewall and network configuration
- Verify SSL/TLS setup for HTTPS endpoints

### Protocol Errors
- Confirm your MCP server supports the expected protocol version
- Check for proper JSON-RPC message formatting
- Review SSE connection handling

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