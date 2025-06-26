# Traefik Integration Guide

This guide provides sample configurations for deploying the Remote MCP Proxy with Traefik in different scenarios.

## Table of Contents

1. [Standalone Deployment](#standalone-deployment) - Complete Traefik + MCP Proxy setup
2. [Existing Traefik Integration](#existing-traefik-integration) - Add to existing Traefik
3. [Configuration Options](#configuration-options) - Customize your setup
4. [Troubleshooting](#troubleshooting) - Common issues and solutions

## Standalone Deployment

If you don't have Traefik running yet, use this complete setup:

### Quick Start

1. **Copy the configuration files:**
   ```bash
   cp traefik/.env.example traefik/.env
   # Edit traefik/.env with your domain and email
   ```

2. **Configure your domain:**
   ```bash
   # Edit traefik/.env
   DOMAIN=yourdomain.com
   ACME_EMAIL=admin@yourdomain.com
   ```

3. **Deploy the stack:**
   ```bash
   cd traefik
   docker-compose -f docker-compose.standalone.yml up -d
   ```

4. **Verify deployment:**
   ```bash
   # Check container health
   docker-compose -f docker-compose.standalone.yml ps
   
   # Wait for healthy status, then test
   curl -k https://mcp.yourdomain.com/health
   ```

### What You Get

With the standalone deployment, you'll have:

- **Traefik Dashboard:** `https://traefik.yourdomain.com`
- **MCP Health Check:** `https://mcp.yourdomain.com/health`
- **Dynamic MCP Routing:** `https://[server-name].mcp.yourdomain.com/sse`

## Existing Traefik Integration

If you already have Traefik running, follow these steps:

### Option 1: Use Existing Docker Compose

Add the remote-mcp-proxy service to your existing docker-compose.yml:

```yaml
services:
  remote-mcp-proxy:
    image: remote-mcp-proxy:latest
    build: 
      context: /path/to/remote-mcp-proxy
      dockerfile: Dockerfile
    container_name: remote-mcp-proxy
    restart: unless-stopped
    volumes:
      - /path/to/config.json:/app/config.json:ro
    read_only: true
    tmpfs:
      - /tmp:exec
      - /root/.npm:exec
    cap_drop:
      - ALL
    security_opt:
      - no-new-privileges:true
    environment:
      - GO_ENV=production
      - DOMAIN=${DOMAIN}
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
    networks:
      - your-traefik-network  # Use your existing Traefik network
    labels:
      - traefik.enable=true
      - traefik.docker.network=your-traefik-network
      
      # Dynamic routing for any MCP server
      - traefik.http.routers.mcp-proxy.rule=HostRegexp(`{subdomain:[a-zA-Z0-9-]+}.mcp.${DOMAIN}`)
      - traefik.http.routers.mcp-proxy.entrypoints=websecure
      - traefik.http.routers.mcp-proxy.tls=true
      - traefik.http.routers.mcp-proxy.tls.certresolver=your-cert-resolver
      - traefik.http.routers.mcp-proxy.service=mcp-proxy-service
      - traefik.http.services.mcp-proxy-service.loadbalancer.server.port=8080
```

### Option 2: Separate Docker Compose

Use the provided `docker-compose.yml` (modify the network name):

```bash
# Edit docker-compose.yml to use your existing network
networks:
  proxy:
    external: true
    name: your-traefik-network  # Change this to your network name
```

Then deploy:
```bash
docker-compose up -d
```

## Configuration Options

### Static vs Dynamic Routing

The configuration supports two routing approaches:

#### 1. Dynamic Routing (Recommended)
```yaml
# Routes any subdomain.mcp.yourdomain.com to the proxy
- traefik.http.routers.mcp-proxy.rule=HostRegexp(`{subdomain:[a-zA-Z0-9-]+}.mcp.${DOMAIN}`)
```

**Benefits:**
- No need to restart Traefik when adding new MCP servers
- Automatically handles any server name defined in config.json
- Cleaner configuration

#### 2. Static Routing (Per-Server)
This is what's in the main docker-compose.yml - specific routes for each server:

```yaml
# filesystem MCP server routing
- traefik.http.routers.filesystem-mcp.rule=Host(`filesystem.mcp.${DOMAIN}`)
- traefik.http.routers.filesystem-mcp.service=filesystem-mcp-service
- traefik.http.services.filesystem-mcp-service.loadbalancer.server.port=8080
```

**Benefits:**
- More explicit control over routing
- Can set different middleware per server
- Better for complex routing rules

### SSL/TLS Configuration

#### HTTP Challenge (Default)
Suitable for most setups:
```yaml
certificatesResolvers:
  myresolver:
    acme:
      httpChallenge:
        entryPoint: web
```

#### DNS Challenge (Wildcard Certificates)
For wildcard certificates, configure DNS challenge:

```yaml
certificatesResolvers:
  myresolver:
    acme:
      dnsChallenge:
        provider: cloudflare  # or your DNS provider
        resolvers:
          - "1.1.1.1:53"
```

Set these environment variables:
```bash
CF_API_EMAIL=your-email@example.com
CF_API_KEY=your-api-key
```

### Middleware Configuration

Add security and CORS middleware by including these labels:

```yaml
labels:
  - traefik.http.routers.mcp-proxy.middlewares=mcp-cors,secure-headers
```

Configure middleware in `dynamic.yml` or via labels:

```yaml
# CORS middleware
- traefik.http.middlewares.mcp-cors.headers.accesscontrolalloworiginlist=*
- traefik.http.middlewares.mcp-cors.headers.accesscontrolallowheaders=*
- traefik.http.middlewares.mcp-cors.headers.accesscontrolallowmethods=GET,POST,PUT,DELETE,OPTIONS
```

## Troubleshooting

### Common Issues

#### 1. Container Not Healthy
```bash
# Check container logs
docker logs remote-mcp-proxy

# Verify health endpoint directly
docker exec remote-mcp-proxy curl -f http://localhost:8080/health
```

#### 2. SSL Certificate Issues
```bash
# Check Traefik logs for ACME errors
docker logs traefik 2>&1 | grep -i acme

# Verify domain DNS resolution
nslookup yourdomain.com

# Test HTTP challenge path
curl http://yourdomain.com/.well-known/acme-challenge/test
```

#### 3. Network Connectivity
```bash
# List Docker networks
docker network ls

# Inspect network configuration
docker network inspect proxy

# Verify containers are on the same network
docker inspect remote-mcp-proxy | grep NetworkMode
```

#### 4. Routing Issues
```bash
# Check Traefik dashboard for router status
# Visit: http://localhost:8080 (or your Traefik dashboard URL)

# Test routing with curl
curl -H "Host: memory.mcp.yourdomain.com" http://localhost/health
```

### Health Check Verification

Always verify the container is healthy before testing external URLs:

```bash
# Wait for healthy status
docker-compose ps

# Should show (healthy) next to remote-mcp-proxy
# If not healthy, check logs:
docker logs remote-mcp-proxy
```

### Performance Considerations

- **Connection Limits:** The proxy handles multiple simultaneous connections
- **Request Queuing:** Requests to the same MCP server are serialized
- **Health Checks:** 30-second intervals with 10-second timeout
- **Cleanup:** Automatic detection of client disconnections within 30 seconds

### Security Best Practices

1. **Use HTTPS in production** - Always configure proper SSL certificates
2. **Secure Traefik dashboard** - Don't expose the API insecurely in production
3. **Network isolation** - Use dedicated Docker networks
4. **Container security** - The proxy runs with minimal privileges (read-only, no-new-privileges)
5. **CORS configuration** - Configure appropriate CORS policies for your use case

For more detailed troubleshooting, see the main [troubleshooting documentation](../docs/troubleshooting.md).