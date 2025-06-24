# Remote MCP Proxy

**Seamlessly use your favorite MCP servers anywhere**. This project packages a small Go proxy that lets you connect local or experimental MCP servers to Claude.ai and the mobile app. Even if a server isn't officially "remote" yet, this proxy exposes it over Claude's new Remote MCP protocol so you can start integrating immediately.

## Why This Exists

Existing MCP servers often run only on your desktop, making them impossible to use with Claude's web UI or phone app. The Remote MCP protocol solves this, but not every server supports it yet. This proxy fills that gap so you can experiment right away.

## How It Works

Run the proxy in Docker and it will:
- Launches and monitors your local MCP servers automatically
- Converts traffic between HTTP/SSE and standard MCP JSON-RPC
- Hosts several MCP servers at once under different URL paths
- Reuses the familiar `claude_desktop_config.json` format
- Shuts down cleanly and cleans up any spawned processes
- Exposes a `/health` endpoint so you can check status at a glance

## 🚀 Dynamic Configuration System

This proxy now features **automatic subdomain routing generation** from your `config.json` file. Simply define your MCP servers in JSON, and the system automatically creates Traefik routing rules for each server.

### ⚡ Super Quick Start
```bash
# 1. Define servers
echo '{"mcpServers":{"memory":{"command":"npx","args":["-y","@modelcontextprotocol/server-memory"]}}}' > config.json

# 2. Set domain  
echo "DOMAIN=yourdomain.com" > .env

# 3. Deploy
make install-deps && make up

# 4. Use in Claude.ai
# → https://memory.mcp.yourdomain.com/sse
```

### Key Features

- ✅ **Dynamic Subdomain Routing**: Each server gets `{server}.mcp.{domain}/sse`
- ✅ **Automatic Traefik Integration**: Routes generated automatically 
- ✅ **Easy Scaling**: Add servers by editing JSON only
- ✅ **Production Ready**: Proper SSL, load balancing, service discovery

## Quick Start

### 1. Create Configuration File

Create a `config.json` file describing your MCP servers (same format as `claude_desktop_config.json`):

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

### 2. Deploy with Dynamic Configuration

**Option A: Automated Make Workflow (Recommended)**

```bash
# Install dependencies (first time only)
make install-deps

# Set your domain
echo "DOMAIN=yourdomain.com" > .env

# Generate configuration and deploy
make up

# View logs
make logs
```

**Option B: Manual Docker Deployment**

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

### 5. Configure DNS Wildcard (Required)

Configure wildcard DNS for dynamic subdomain routing:

**DNS Setup Example (Cloudflare)**:
```dns
Type: A
Name: *.mcp
Content: YOUR_SERVER_IP
Proxy status: Proxied (orange cloud)
```

**For other DNS providers**, create an A record:
```dns
*.mcp.your-domain.com    A    YOUR_SERVER_IP
```

### 6. Configure Claude.ai

Open Claude.ai (requires Pro, Max, Teams, or Enterprise plan) and add your **automatically generated** proxy URLs under Settings > Integrations:

**Auto-Generated URLs** (based on your config.json):
 - `https://notion-mcp.mcp.your-domain.com/sse`
 - `https://memory-mcp.mcp.your-domain.com/sse`
 - `https://sequential-thinking.mcp.your-domain.com/sse`

✅ **Claude.ai Integration Status**: The Connect button now works reliably! The proxy fully supports Claude.ai Remote MCP integration with proper session management and tool discovery.

### 🔄 Adding New MCP Servers

**1. Edit config.json:**
```json
{
  "mcpServers": {
    "existing-server": {...},
    "new-server": {
      "command": "python",
      "args": ["/path/to/server.py"]
    }
  }
}
```

**2. Redeploy:**
```bash
make restart
```

**3. Use immediately:**
- New URL: `https://new-server.mcp.your-domain.com/sse`
- Automatically configured SSL, routing, load balancing

**Debug Endpoints**: Use these endpoints to verify your MCP servers are working:
- Check server status: `https://mcp.your-domain.com/listmcp`
- Verify tools available: `https://mcp.your-domain.com/listtools/your-server-name`

## 🌐 Dynamic URL Structure

**Auto-Generated Format**: Each MCP server is automatically available at:
```
https://{server-name}.mcp.{DOMAIN}/sse
```

**Examples** (from your config.json):
- `https://memory.mcp.your-domain.com/sse`
- `https://sequential-thinking.mcp.your-domain.com/sse`
- `https://notion.mcp.your-domain.com/sse`

Where `{DOMAIN}` is set in your `.env` file and `{server-name}` matches the key in your `config.json` file.

### 🔧 Make Commands Reference

| Command | Description |
|---------|-------------|
| `make help` | Show all available commands |
| `make install-deps` | Install gomplate dependency |
| `make generate` | Generate docker-compose.yml from config.json |
| `make build` | Build Docker images |
| `make up` | Generate config and start services |
| `make down` | Stop and remove services |
| `make restart` | Restart services with new config |
| `make logs` | Show service logs |
| `make clean` | Remove generated files |

### Why Dynamic Subdomain Generation?

Claude.ai expects Remote MCP endpoints at root level (`/sse`), not path-based routing. This automated subdomain approach:
- ✅ Matches Remote MCP standard format
- ✅ Auto-scales with config.json changes
- ✅ Provides clean separation between servers
- ✅ Eliminates manual Traefik configuration
- ✅ Enables instant deployment of new servers

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

- **`DOMAIN`**: Your base domain name (e.g., `example.com`). MCP servers will be accessible at `{server}.mcp.{DOMAIN}`

#### MCP Server Environment Variables

- Set environment variables for your MCP servers in the `env` section of `config.json`
- Store secrets securely and reference them in your Docker deployment
- The proxy will pass these environment variables to the spawned MCP processes

## Docker Compose with Traefik

### Wildcard Subdomain Configuration

The service is configured to work with Traefik reverse proxy for automatic HTTPS and **wildcard subdomain** routing:

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
      # Wildcard subdomain routing for dynamic MCP servers
      - traefik.enable=true
      - traefik.http.routers.mcp-wildcard.rule=Host(`*.mcp.${DOMAIN}`)
      - traefik.http.routers.mcp-wildcard.entrypoints=websecure
      - traefik.http.routers.mcp-wildcard.tls=true
      - traefik.http.routers.mcp-wildcard.tls.certresolver=letsencrypt
      - traefik.http.services.mcp-wildcard.loadbalancer.server.port=8080
      
      # Utility endpoints on main domain
      - traefik.http.routers.mcp-main.rule=Host(`mcp.${DOMAIN}`)
      - traefik.http.routers.mcp-main.entrypoints=websecure
      - traefik.http.routers.mcp-main.tls=true
      - traefik.http.routers.mcp-main.tls.certresolver=letsencrypt
      - traefik.http.services.mcp-main.loadbalancer.server.port=8080

networks:
  proxy:
    external: true
```

### Key Configuration Points:

1. **Wildcard Rule**: `Host(\`*.mcp.${DOMAIN}\`)` captures all subdomains like `memory.mcp.domain.com`
2. **Dynamic SSL**: Traefik automatically generates SSL certificates for new subdomains
3. **Main Domain**: `mcp.${DOMAIN}` for utility endpoints (`/health`, `/listmcp`)
4. **DNS Requirement**: Wildcard DNS record `*.mcp.domain.com` must be configured

## Complete Setup Guide

### Prerequisites

- Docker and Docker Compose installed
- Domain name with DNS control
- Traefik reverse proxy running (or willingness to set it up)

### Step-by-Step Setup

#### 1. Clone and Configure

```bash
# Clone the repository
git clone <repository-url>
cd remote-mcp-proxy

# Create environment configuration
echo "DOMAIN=your-domain.com" > .env
```

#### 2. Configure Your MCP Servers

Edit `config.json` with your desired MCP servers:

```json
{
  "mcpServers": {
    "memory": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-memory"]
    },
    "sequential-thinking": {
      "command": "npx", 
      "args": ["-y", "@modelcontextprotocol/server-sequential-thinking"]
    },
    "notion": {
      "command": "npx",
      "args": ["-y", "@anthropic-ai/mcp-server-notion"],
      "env": {
        "NOTION_TOKEN": "your_notion_token_here"
      }
    }
  }
}
```

#### 3. Configure DNS (Critical Step)

**For Cloudflare:**
1. Go to DNS settings for your domain
2. Add new record:
   - **Type**: A
   - **Name**: `*.mcp`
   - **Content**: Your server's IP address
   - **Proxy status**: Proxied (orange cloud)

**For other DNS providers:**
Create a wildcard A record: `*.mcp.your-domain.com → YOUR_SERVER_IP`

#### 4. Set Up Traefik (If Not Already Running)

Create `traefik/docker-compose.yml`:

```yaml
version: '3.8'
services:
  traefik:
    image: traefik:v3.0
    container_name: traefik
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    networks:
      - proxy
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ./traefik.yml:/traefik.yml:ro
      - ./acme.json:/acme.json
    environment:
      - CF_API_EMAIL=your-email@example.com  # If using Cloudflare
      - CF_API_KEY=your-cloudflare-api-key   # If using Cloudflare

networks:
  proxy:
    external: true
```

Create `traefik/traefik.yml`:

```yaml
global:
  checkNewVersion: false

entryPoints:
  web:
    address: ":80"
  websecure:
    address: ":443"

providers:
  docker:
    endpoint: "unix:///var/run/docker.sock"
    exposedByDefault: false

certificatesResolvers:
  letsencrypt:
    acme:
      email: your-email@example.com
      storage: acme.json
      dnsChallenge:  # Recommended for wildcard certificates
        provider: cloudflare
        delayBeforeCheck: 0
```

#### 5. Deploy the MCP Proxy

```bash
# Create proxy network (if not exists)
docker network create proxy

# Start Traefik (if not running)
cd traefik && docker-compose up -d && cd ..

# Deploy MCP Proxy
docker-compose up -d
```

#### 6. Verify Deployment

```bash
# Check if services are running
docker-compose ps

# Test main endpoints
curl -s https://mcp.your-domain.com/health
curl -s https://mcp.your-domain.com/listmcp

# Test individual MCP server subdomains
curl -s https://memory.mcp.your-domain.com/health
curl -s https://sequential-thinking.mcp.your-domain.com/health
```

#### 7. Add to Claude.ai

1. Open Claude.ai (requires Pro/Team/Enterprise plan)
2. Go to Settings → Integrations
3. Click "Add More" → "Custom Integration"
4. Add your MCP server URLs:
   - `https://memory.mcp.your-domain.com/sse`
   - `https://sequential-thinking.mcp.your-domain.com/sse`
   - `https://notion.mcp.your-domain.com/sse`

### Troubleshooting

#### DNS Issues
```bash
# Test DNS resolution
nslookup memory.mcp.your-domain.com
dig *.mcp.your-domain.com

# Should resolve to your server IP
```

#### SSL Certificate Issues
```bash
# Check Traefik logs
docker logs traefik

# Check certificate generation
docker exec traefik cat /acme.json
```

#### MCP Server Issues
```bash
# Check proxy logs
docker logs remote-mcp-proxy

# Test individual server tools
curl -s https://mcp.your-domain.com/listtools/memory
```

#### Claude.ai Connection Issues
1. Verify URL format: `https://server.mcp.domain.com/sse`
2. Check authentication (if required)
3. Ensure DNS and SSL are working
4. Test with browser first

### Environment Variables

- **`DOMAIN`**: Your base domain (required)
- **`MCP_DOMAIN`**: Override domain for MCP routing (optional)
- **`PORT`**: HTTP server port (default: 8080)

### Dynamic Configuration Commands

```bash
# View current servers
jq '.mcpServers | keys' config.json

# Generate and view routing configuration  
make generate
cat docker-compose.yml

# View logs for all services
make logs

# Quick restart after config changes
make restart

# Add new MCP server workflow:
# 1. Edit config.json - add new server
# 2. Run: make restart
# 3. New URL automatically available: https://newserver.mcp.domain.com/sse
# 4. All SSL, routing, service discovery handled automatically

# Update to latest version
docker-compose pull && make up
```

### 🏗️ Technical Architecture

```
config.json → gomplate → docker-compose.yml → Traefik → Claude.ai
     ↓            ↓              ↓               ↓          ↓
   Servers    Templates    Container Labels  SSL Routes  Integration
```

**Workflow**:
1. **config.json**: Define MCP servers (single source of truth)
2. **gomplate**: Template engine generates docker-compose.yml
3. **Traefik Labels**: Each server gets automatic routing rules
4. **SSL**: Automatic certificate generation for subdomains
5. **Claude.ai**: Ready-to-use URLs with zero manual configuration

### 📁 Dynamic Configuration Files

```
remote-mcp-proxy/
├── config.json                    # ← MCP server definitions (edit this)
├── .env                          # ← Domain configuration
├── docker-compose.yml.template   # ← Template for generation
├── docker-compose.yml           # ← Generated automatically (don't edit)
├── Makefile                     # ← Build automation
└── ...
```

**Key Files:**
- **Edit**: `config.json`, `.env` 
- **Auto-generated**: `docker-compose.yml`
- **Use**: `make` commands for all operations

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

### Remote MCP Protocol Implementation

The proxy implements the Remote MCP protocol specification to enable Claude.ai integration:

#### Protocol Flow
1. **OAuth 2.0 Authentication**: Claude.ai authenticates using Bearer tokens via OAuth 2.0 Dynamic Client Registration
2. **Initialize Handshake**: Synchronous POST request to `/{server}/sse` with initialize message
3. **Session Management**: Sessions are tracked using `Mcp-Session-Id` header and marked as initialized immediately after successful handshake
4. **Tool Discovery**: Follow-up requests use the same session to discover and call tools
5. **SSE Communication**: Server-Sent Events for real-time message delivery (future requests)

#### Critical Implementation Details

**Synchronous Initialize**: Unlike local MCP servers, Claude.ai expects a synchronous JSON response to the initialize POST request, not an asynchronous SSE response.

**Session Initialization**: Sessions MUST be marked as initialized immediately after a successful MCP server response. Waiting for a separate "initialized" notification will cause tool discovery to fail.

**Stdio Concurrency**: MCP server stdout access is serialized using dedicated `readMu` mutex to prevent deadlocks when multiple Claude.ai requests access the same server simultaneously.

**Timeout Handling**: 30-second timeout for initialize responses to accommodate slow npm-based MCP servers. Shorter timeouts cause "context deadline exceeded" errors.

**Tool Name Normalization**: Tool names are automatically converted from hyphenated format (API-get-user) to snake_case (api_get_user) for Claude.ai compatibility, with bidirectional transformation for tool calls.

## Monitoring and Health Checks

The proxy includes several endpoints for monitoring and debugging:

### Health Check
```bash
# Check proxy health
curl http://localhost:8080/health
# Response: {"status":"healthy"}
```

### List Configured MCP Servers
```bash
# List all configured MCP servers and their status
curl http://localhost:8080/listmcp
# Response: {"count":1,"servers":[{"name":"notionApi","running":true,"pid":11,"command":"npx","args":["-y","@notionhq/notion-mcp-server"]}]}
```

### List Tools for an MCP Server
```bash
# List available tools for a specific MCP server
curl http://localhost:8080/listtools/{server-name}
# Example: curl http://localhost:8080/listtools/notionApi
# Response: {"server":"notionApi","response":{"id":"...","jsonrpc":"2.0","result":{"tools":[...]}}}
```

These endpoints are useful for:
- **Health monitoring**: Verify the proxy is running
- **Configuration debugging**: Check which MCP servers are configured and running
- **Tool discovery**: See what tools are available from each MCP server
- **Troubleshooting**: Identify if MCP servers are properly started and responding

## Troubleshooting

### Claude.ai "Connect" Button Issues (RESOLVED ✅)

**Issue**: The Connect button in Claude.ai Remote MCP settings appears to work but then fails, or shows "context deadline exceeded" errors.

**Root Cause**: This was caused by stdio deadlocks during MCP server initialization handshake and improper session management.

**Resolution**: These critical issues have been resolved in the current version:

1. **Stdio Deadlock Fix**: Added dedicated `readMu` mutex to prevent race conditions when multiple requests access the same MCP server stdout
2. **Session Initialization Fix**: Sessions are now properly marked as initialized after successful handshake
3. **Timeout Adjustment**: Increased initialize timeout from 10 to 30 seconds for slow npm-based MCP servers

**Verification**: 
- The Connect button should now work reliably
- Tools should be properly exposed and usable in Claude.ai
- Check logs for "Session marked as initialized" messages

### MCP Server Won't Start
- Check the command and arguments in your config
- Verify environment variables are set correctly
- Look at proxy logs for process spawn errors
- Ensure required dependencies are available in the container

**Common npm-based MCP server issues**:
```bash
# Check if npm packages are available
docker exec remote-mcp-proxy npm list -g

# Verify MCP server can start manually
docker exec -it remote-mcp-proxy npx -y @notionhq/notion-mcp-server
```

### Connection Issues
- Ensure the proxy is accessible from Claude.ai
- Check that Traefik is properly configured with SSL certificates
- Verify the domain DNS points to your server
- Ensure port 80/443 are open in your firewall

### "Context Deadline Exceeded" Errors (RESOLVED ✅)

**Issue**: Logs show "context deadline exceeded" during initialize handshake.

**Root Cause**: This was caused by stdio deadlocks and insufficient timeout for MCP server initialization.

**Resolution**: Fixed in current version with dedicated read mutex and increased timeouts.

**If still occurring**:
- Check if MCP server processes are actually running: `docker exec remote-mcp-proxy ps aux`
- Verify MCP server responds to direct communication: `docker exec -i remote-mcp-proxy npx -y <server> <<< '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}'`

### Session Not Initialized Errors (RESOLVED ✅)

**Issue**: Tools not showing up in Claude.ai even after successful connection.

**Root Cause**: Sessions were not being marked as initialized after successful handshake.

**Resolution**: Fixed - sessions are now automatically marked as initialized when the MCP server responds successfully to the initialize request.

### Tools Not Appearing

If tools don't appear after successful connection:

1. **Check MCP server tools**: 
   ```bash
   curl https://mcp.your-domain.com/listtools/your-server-name
   ```

2. **Verify tool name normalization**: Tool names are automatically converted to snake_case for Claude.ai compatibility

3. **Check server capabilities**: Some MCP servers may not expose tools immediately after startup

### General Connection Debugging

- Check firewall and network configuration
- Verify SSL/TLS setup for HTTPS endpoints  
- Test the SSE endpoint directly: `curl http://localhost:8080/{server-name}/sse`
- Use the monitoring endpoints to debug:
  - Check if MCP servers are running: `curl http://localhost:8080/listmcp`
  - Verify tools are available: `curl http://localhost:8080/listtools/{server-name}`

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
