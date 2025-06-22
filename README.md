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

### 3. Setup Environment Variables

Create a `.env` file with your domain configuration:

```bash
# Copy example file
cp .env.example .env

# Edit with your domain
echo "DOMAIN=your-domain.com" > .env
```

### 4. Use Docker Compose with Traefik

```bash
docker-compose up -d
```

This will deploy the service with Traefik reverse proxy integration, making it accessible at `mcp.{DOMAIN}` with automatic HTTPS.

### 5. Configure Claude.ai

In Claude's web UI, add your remote MCP servers using these URLs:
- `https://mcp.your-domain.com/notion-mcp/sse`
- `https://mcp.your-domain.com/memory-mcp/sse`

Replace `your-domain.com` with your actual domain configured in the `.env` file.

## URL Structure

Each MCP server is available at:
```
https://mcp.{DOMAIN}/{server-name}/sse
```

Where `{DOMAIN}` is set in your `.env` file and `{server-name}` matches the key in your `config.json` file.

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

#### Docker Compose Environment Variables

The following environment variables are used by the Docker Compose setup:

- **`DOMAIN`**: Your base domain name (e.g., `example.com`). The service will be accessible at `mcp.{DOMAIN}`

#### MCP Server Environment Variables

- Set environment variables for your MCP servers in the `env` section of `config.json`
- Store secrets securely and reference them in your Docker deployment
- The proxy will pass these environment variables to the spawned MCP processes

## Docker Compose with Traefik

The service is configured to work with Traefik reverse proxy for automatic HTTPS and domain routing:

```yaml
version: '3.8'
services:
  remote-mcp-proxy:
    build: .
    container_name: remote-mcp-proxy
    restart: unless-stopped
    volumes:
      - ./config.json:/app/config.json:ro
    environment:
      - GO_ENV=production
    networks:
      - proxy
    labels:
      - traefik.enable=true
      - traefik.http.routers.remote-mcp-proxy.rule=Host(`mcp.${DOMAIN}`)
      - traefik.http.routers.remote-mcp-proxy.entrypoints=websecure
      - traefik.http.routers.remote-mcp-proxy.tls.certresolver=myresolver
networks:
  proxy:
    external: true
```

Make sure to:
1. Create a `.env` file with your `DOMAIN` variable
2. Have Traefik running with the `proxy` network
3. Configure SSL certificate resolver in Traefik

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

### Testing

The Remote MCP Proxy includes comprehensive tests to ensure reliability and correctness.

#### Quick Test

```bash
# Run all tests
./test/run-tests.sh
```

#### Manual Testing

```bash
# Unit tests only
go test -v ./protocol ./mcp ./proxy

# Integration tests
go test -v .

# Tests with coverage
go test -cover ./...

# Short tests (skip integration)
go test -short ./...

# Benchmarks
go test -bench=. -benchmem ./...
```

#### Test Configurations

Several test configurations are provided in the `test/` directory:

- **`test/minimal-config.json`**: Basic echo server for testing
- **`test/development-config.json`**: Common MCP servers for development  
- **`test/production-config.json`**: Production server examples
- **`test/config.json`**: Full test suite configuration

#### Testing with Different Configurations

```bash
# Test with minimal config
CONFIG_PATH=./test/minimal-config.json ./remote-mcp-proxy

# Test with development servers (requires npm packages)
CONFIG_PATH=./test/development-config.json ./remote-mcp-proxy

# Test specific functionality
curl http://localhost:8080/health
curl -X GET http://localhost:8080/simple-echo/sse \
  -H "Accept: text/event-stream"
```

#### Test Coverage

The test suite covers:

- **Protocol Translation**: JSON-RPC ↔ Remote MCP message conversion
- **Connection Management**: Session handling, timeouts, cleanup
- **Error Handling**: Invalid requests, server failures, network issues
- **Concurrency**: Multiple simultaneous connections
- **Authentication**: Token validation and CORS
- **Health Checks**: Server status monitoring
- **Integration**: End-to-end workflow testing

#### CI/CD Testing

For automated testing in CI environments:

```bash
# Install dependencies
go mod download

# Run tests with XML output (for CI)
go test -v ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html

# Static analysis
go vet ./...
go fmt ./...
```

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