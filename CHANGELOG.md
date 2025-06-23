# Changelog

All notable changes to the Remote MCP Proxy project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Comprehensive Changelog Management**: Automated changelog workflow integrated into CLAUDE.md development guidelines
- **Traefik Session Persistence**: Sticky sessions, health checks, and SSE-optimized headers for better connection reliability
- **Development Protocol**: Structured commit message conventions and documentation management process

### Fixed
- **Critical Session Management Bug**: Fixed session ID handling to properly use Claude.ai's `Mcp-Session-Id` header instead of generating new sessions for each request
- **SSE Connection Coordination**: Ensured SSE connections use the same session management as POST requests for proper state persistence

### Changed
- **Docker Compose Configuration**: Enhanced with Traefik optimizations including CORS headers, connection persistence, and resource management
- **CLAUDE.md Guidelines**: Added mandatory changelog management protocol and automated commit preparation workflow

## [1.2.0] - 2025-06-23

### Added
- **Remote MCP Protocol Compliance**: Complete implementation of Remote MCP protocol with proper SSE endpoints
- **Session Management**: Full session lifecycle management with `Mcp-Session-Id` header support
- **Comprehensive Logging**: Detailed request/response logging for debugging Claude.ai integration issues
- **Debug Endpoints**: `/listmcp` and `/listtools/{server}` endpoints for troubleshooting
- **Traefik Optimizations**: SSE-specific Traefik configuration for better persistence and reliability

### Fixed
- **Critical Session Bug**: Fixed session ID generation to use Claude.ai's `Mcp-Session-Id` header instead of generating new ones
- **SSE Connection Handling**: Proper coordination between SSE connections and POST message sessions
- **Protocol Handshake**: Correct implementation of initialize/initialized sequence
- **Connection Management**: Proper cleanup and lifecycle management for SSE connections

### Changed
- **Docker Compose**: Enhanced with Traefik sticky sessions, health checks, and CORS headers
- **Error Handling**: Comprehensive try/catch patterns with detailed success/failure logging
- **Authentication**: Improved header validation and session state management

## [1.1.0] - 2025-06-22

### Added
- **Server Status Endpoints**: Monitoring endpoints for server health and tool discovery
- **Test Binary**: Added test-binary for development and debugging
- **Health Check Integration**: Docker health checks with proper curl commands

### Fixed
- **Docker Configuration**: Removed config.json from .dockerignore for proper build context
- **Health Check Command**: Changed from wget to curl for Alpine Linux compatibility

### Changed
- **Documentation**: Enhanced README with monitoring endpoints and troubleshooting guide
- **PRD Updates**: Documented critical Remote MCP protocol issues and implementation gaps

## [1.0.0] - 2025-06-21

### Added
- **Initial Implementation**: Complete Remote MCP Proxy service
- **MCP Server Management**: Process lifecycle management for local MCP servers
- **Protocol Translation**: JSON-RPC ↔ Remote MCP message translation
- **SSE Support**: Server-Sent Events for real-time communication
- **Docker Support**: Multi-stage Docker build with Alpine Linux
- **Traefik Integration**: Reverse proxy support with automatic HTTPS
- **Configuration Management**: claude_desktop_config.json format support
- **Testing Framework**: Comprehensive test suite with multiple configurations

### Infrastructure
- **Docker Compose**: Production-ready deployment configuration
- **Environment Management**: .env file support for domain configuration
- **Health Monitoring**: Built-in health check endpoints
- **Resource Management**: CPU and memory limits for container deployment

### Documentation
- **README**: Comprehensive setup and usage documentation
- **PRD**: Product Requirements Document with architecture details
- **CLAUDE.md**: Development guidelines and coding standards

### Dependencies
- **Go Runtime**: Go 1.21+ with Gorilla Mux for HTTP routing
- **Docker**: Alpine Linux base with Node.js and Python support
- **MCP Servers**: Support for npm and Python-based MCP servers

## [0.1.0] - 2025-06-20

### Added
- **Project Initialization**: Basic project structure and Git setup
- **Core Architecture**: Foundation for Remote MCP proxy functionality

---

## Version History Summary

- **v1.2.0**: Remote MCP protocol compliance with session management fixes
- **v1.1.0**: Monitoring endpoints and Docker improvements  
- **v1.0.0**: Complete initial implementation with Docker and Traefik support
- **v0.1.0**: Project foundation

---

## Development Notes

### Commit Message Conventions
- `feat:` - New features
- `fix:` - Bug fixes
- `docs:` - Documentation changes
- `refactor:` - Code refactoring
- `test:` - Test additions/modifications
- `chore:` - Maintenance tasks

### Breaking Changes
Breaking changes are documented in the release notes with migration instructions.

### Security Updates
Security-related changes are marked with 🔒 and include CVE references when applicable.