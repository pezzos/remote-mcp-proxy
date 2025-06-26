# Traefik Configuration for Remote MCP Proxy

This directory contains advanced Traefik configurations for the Remote MCP Proxy service.

## Quick Start (Recommended)

The easiest way to deploy is using the integrated approach:

1. **Configure environment in project root:**
   ```bash
   cd ..  # Go to project root
   cp .env.example .env
   # Edit .env: set ENABLE_LOCAL_TRAEFIK=true and configure DOMAIN, ACME_EMAIL
   ```

2. **Deploy using make:**
   ```bash
   make up
   ```

3. **Access your services:**
   - Traefik Dashboard: `https://traefik.yourdomain.com`
   - MCP Health: `https://mcp.yourdomain.com/health`
   - MCP Servers: `https://[server-name].mcp.yourdomain.com/sse`

## Advanced Configuration

This directory contains additional configuration files for complex deployments:

## Files Overview

- `traefik.yml` - Static Traefik configuration with DNS challenge support
- `dynamic.yml` - Dynamic configuration with middleware and security headers
- `INTEGRATION.md` - Detailed integration guide for complex scenarios
- `README.md` - This file

## Key Features

- **Dynamic MCP Routing:** Automatically routes `*.mcp.yourdomain.com` to appropriate MCP servers
- **SSL/TLS Support:** Automatic Let's Encrypt certificates via HTTP or DNS challenge
- **Security:** CORS headers, security headers, and rate limiting middleware
- **Health Monitoring:** Integrated health checks and status monitoring
- **Production Ready:** Secure defaults and hardened configuration

## Integration Options

1. **Simple Integration** (recommended):
   - Use `ENABLE_LOCAL_TRAEFIK=true` in main `.env`
   - Deploy with `make up` from project root

2. **Advanced Integration**:
   - Use files in this directory with existing Traefik setups
   - See [INTEGRATION.md](INTEGRATION.md) for detailed instructions

## Support

For issues and troubleshooting, see:
- [INTEGRATION.md](INTEGRATION.md) - Advanced integration scenarios
- [../docs/troubleshooting.md](../docs/troubleshooting.md) - General troubleshooting