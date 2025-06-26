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

## Investigation Command

### `/investigate` - Systematic Problem Analysis

**Purpose**: Launch systematic investigation mode for complex technical issues requiring methodical analysis and documentation.

**Usage**: `/investigate [problem-description]`

### `/reflect` - Session Documentation and Knowledge Preservation

**Purpose**: Document session achievements, solutions implemented, and lessons learned for future reference and knowledge preservation.

**Usage**: `/reflect [optional-session-focus]`

**When to use `/investigate`**:
- Complex bugs affecting multiple system components
- Protocol compliance issues requiring systematic analysis
- Performance problems needing methodical investigation
- Integration failures with unclear root causes
- Recurring issues that need pattern analysis
- Multi-server or multi-layer system problems

**When to use `/reflect`**:
- End of significant development sessions
- After implementing major features or fixes
- Following successful problem resolution
- When documentation needs updating with new knowledge
- Before switching to different project areas
- To create permanent record of session achievements

**Investigation Workflow Process**:
1. **Problem Definition** - Document clear problem statement and observable symptoms
2. **Evidence Gathering** - Use TodoWrite to track investigation phases, collect logs, test hypotheses
3. **Root Cause Analysis** - Analyze patterns, cross-reference with existing documentation
4. **Solution Implementation** - Implement fixes based on root cause analysis
5. **Documentation** - Update INVESTIGATIONS.md with findings and solutions

**Reflection Workflow Process**:
1. **Session Summary** - Review major accomplishments and changes made
2. **Solution Documentation** - Record specific fixes, configurations, and code changes
3. **Knowledge Capture** - Document lessons learned and breakthrough insights
4. **Process Updates** - Update CLAUDE.md and documentation with new procedures
5. **Future Reference** - Create searchable record for similar future issues

**Investigation Structure**:
- Creates TodoWrite list breaking down investigation into phases
- **REQUIRED**: Updates or creates INVESTIGATIONS.md with structured documentation
- Establishes success criteria for resolution
- Documents breakthrough findings and dead ends systematically
- Ensures knowledge preservation for future reference

**Reflection Structure**:
- Creates or updates SESSION-NOTES.md with session achievements
- Documents specific technical solutions and their context
- Records configuration changes and their rationale
- Updates relevant documentation files (CLAUDE.md, troubleshooting guides)
- Preserves knowledge for future sessions and team members

**SESSION-NOTES.md Format**:
```markdown
# Session Notes: [Date] - [Brief Description]
**Date**: [YYYY-MM-DD]
**Duration**: [Time spent]
**Focus**: [Main areas worked on]

## Achievements
- [Bullet point of major accomplishment]
- [Another achievement with technical details]

## Technical Changes Made
### [Component/File Modified]
- **Change**: [Description of what was changed]
- **Rationale**: [Why this change was needed]
- **Impact**: [Effect of the change]

## Lessons Learned
- [Key insight or best practice discovered]
- [Process improvement or technique learned]

## Updated Documentation
- [File]: [What was updated and why]
- [Reference]: [Links to new or updated docs]

## Future Considerations
- [Potential improvements identified]
- [Areas that may need attention later]
```

**INVESTIGATIONS.md Format**:
```markdown
# Investigation: [Problem Title]
**Date**: [YYYY-MM-DD]
**Status**: [In Progress/Resolved/Blocked]

## Problem Statement
[Clear description of the issue and observable symptoms]

## Evidence Gathered
- [Timestamp]: [Finding/observation]
- [Timestamp]: [Test result or log analysis]

## Root Cause Analysis
[Analysis of patterns and underlying causes]

## Solution Implemented
[Detailed solution steps and configuration changes]

## Verification
[How the solution was tested and confirmed]

## Lessons Learned
[Key insights for future reference]
```

**Investigation Example**:
```bash
/investigate "Claude.ai shows connected but no tools appear"
# Sets up investigation todos, creates INVESTIGATIONS.md section
# Guides through systematic analysis of symptoms, hypotheses, testing
# Documents final solution and updates relevant documentation
```

**Reflection Examples**:
```bash
/reflect "logging cleanup session"
# Documents logging improvements made, code changes, and lessons learned
# Updates SESSION-NOTES.md with technical details and rationale
# Records configuration changes and their impact

/reflect
# General session documentation - reviews all changes made
# Creates comprehensive record of session achievements
# Updates relevant documentation files as needed
```

### Key Development Rules
1. **TodoWrite Usage Protocol**:
   - **REQUIRED for**: Multi-step tasks (3+ steps), architectural changes, debugging complex issues, feature implementations, integration work, investigation workflows
   - **OPTIONAL for**: Single-step tasks, simple file edits, basic queries, straightforward fixes, concurrent tool operations that don't require planning
   - **Balance with Concurrency**: When multiple independent operations can be batched (file reads, parallel searches, multiple bash commands), prioritize concurrent tool usage over TodoWrite planning for efficiency
   - **Examples requiring TodoWrite**: Adding Traefik integration, implementing new MCP servers, fixing multi-component bugs, deployment workflows, systematic investigations
   - **Examples not requiring TodoWrite**: Fixing a typo, adding a comment, reading a single file, answering questions, concurrent file reads, parallel status checks
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
   - **Use Docker MCP tools** for: Container status checks, log retrieval, deployment management, systematic debugging
   - **Use Bash docker commands** for: Interactive operations, complex docker-compose workflows, local debugging
   - **Integration workflow**: Use Docker MCP tools in parallel with other debugging tools for comprehensive system analysis
   - **Preferred for investigations**: Docker MCP tools provide cleaner output and better integration with concurrent tool usage patterns

### Deployment Protocol

#### Dynamic Package Management System
9. **Truly Dynamic Package Detection**: The system achieves zero hardcoded MCP packages through intelligent config.json parsing
   - **Package Discovery**: Automatically extracts package names from `args` arrays in config.json
   - **Multi-Manager Support**: npm (npx), Python (uvx), pip, and direct binary calls
   - **Binary Resolution**: Dynamically discovers binary names by reading installed package.json files
   - **Runtime Optimization**: Converts slow npx calls to fast direct binary execution at container startup

   **Supported Package Manager Formats**:
   ```json
   {
     "mcpServers": {
       "npm-server": {
         "command": "npx",
         "args": ["-y", "@scope/package-name", "arg1", "arg2"]
       },
       "python-server": {
         "command": "uvx", 
         "args": ["python-package-name", "--option"]
       },
       "pip-server": {
         "command": "python",
         "args": ["-m", "installed_package", "config.json"]
       },
       "direct-binary": {
         "command": "pre-installed-binary",
         "args": ["arg1", "arg2"]
       }
     }
   }
   ```

#### MCP Server Storage and Environment Configuration
**Storage Requirements**: Many MCP servers need persistent storage for data, cache, or configuration files.

**Common Storage Patterns**:
- **Memory/Knowledge Graph**: Requires writable file for persistent data storage
- **Database Servers**: Need writable directory for database files  
- **Cache Servers**: Require temporary but persistent cache storage
- **Configuration Servers**: May need to write config or state files

**Environment Variable Patterns**:
```json
{
  "mcpServers": {
    "memory": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-memory"],
      "env": {
        "MEMORY_FILE_PATH": "/app/mcp-data/memory.json"
      }
    },
    "database-server": {
      "command": "npx", 
      "args": ["-y", "@example/database-mcp"],
      "env": {
        "DB_PATH": "/app/mcp-data/database/",
        "DB_NAME": "mcp_storage.db"
      }
    }
  }
}
```

**Storage Volume Requirements**: When MCP servers need persistent storage, add appropriate volumes to docker-compose.yml.template:
```yaml
volumes:
  - mcp-data:/app/mcp-data  # General MCP data storage
  - mcp-cache:/app/mcp-cache  # Temporary but persistent cache
```

10. **Dynamic File Generation**: Both docker-compose.yml and Dockerfile are dynamically generated from templates using `make`
    - **Docker Compose**: `docker-compose.yml.template` → `docker-compose.yml` (auto-created, not tracked in git)
    - **Dockerfile**: `Dockerfile.template` → `Dockerfile` (auto-created, not tracked in git)  
    - **Generation triggers**: Changes to `config.json` or `.env` files
    - **Package Detection Flow**: config.json → parse args → detect packages → generate install commands → optimize Dockerfile
#### Runtime Optimization Process
11. **Startup Optimization Flow**: Container automatically optimizes MCP server execution for maximum performance
    - **Step 1**: `scripts/generate-dockerfile.py` parses config.json and pre-installs packages during build
    - **Step 2**: `scripts/startup.sh` runs at container start to convert npx commands to direct binaries
    - **Step 3**: `scripts/convert-config.py` reads package.json files to discover actual binary names  
    - **Step 4**: Converted config saved to `/tmp/config.json` with direct binary commands
    - **Step 5**: Main application uses optimized config for instant MCP server startup

    **Performance Impact**:
    - **Startup Time**: ~30s → ~3s (90% improvement)
    - **Memory Usage**: 350MB → 113MB (68% reduction) 
    - **Image Size**: 559MB → 254MB (54% reduction)
    - **Process Count**: 109 → 57 PIDs (45% reduction)
    - **No Timeouts**: Eliminates npx installation delays and hangs

12. **Environment Configuration**: Copy and configure `.env` file before deployment:
    ```bash
    cp .env.example .env
    # Required: DOMAIN=yourdomain.com
    # Optional: ENABLE_LOCAL_TRAEFIK=true/false, ACME_EMAIL=admin@yourdomain.com
    ```
#### Deployment Commands
13. **Make Command Workflow**: Understanding the relationship between different generation and deployment commands
    - **`make generate-dockerfile`**: Parse config.json → extract packages → generate optimized Dockerfile only
      - Use when: Testing Dockerfile changes, debugging package detection, iterating on container optimization
      - Output: Creates `Dockerfile` with pre-installed packages based on current config.json
      
    - **`make generate`**: Generate both docker-compose.yml and Dockerfile from templates
      - Use when: Full template regeneration needed, config.json or .env changes, preparing for deployment
      - Output: Creates both `docker-compose.yml` and `Dockerfile` with current configuration
      
    - **`make up`**: Complete deployment workflow (generate → build → deploy)
      - Use when: Starting services, deploying changes, full system restart
      - Process: Calls `make generate` → `docker-compose up --build -d`
      
    - **`make down`**: Stop and remove services only (preserves generated files)
      - Use when: Stopping services temporarily, debugging, maintenance
      
    - **`make clean`**: Remove ALL generated files (docker-compose.yml, Dockerfile)
      - Use when: Fresh start needed, switching configurations, cleaning workspace
      - **Important**: Always run `make generate` after `make clean` before other operations
      
    - **`make logs`**: Show service logs (requires running services)
    - **`make restart`**: Equivalent to `make down && make up`
14. **Secure Volume Management**: Balance security with functionality when adding writable storage
    - **Security Principle**: Container remains `read_only: true` with minimal writable volumes
    - **Required Volumes**: Only add writable volumes when MCP servers explicitly need persistent storage
    - **Volume Scope**: Use specific volume paths (e.g., `/app/mcp-data/`) rather than broad filesystem access
    - **Security Review**: Each new volume should be justified by specific MCP server requirements
    
    **Volume Addition Process**:
    1. **Identify Need**: MCP server documentation or error logs indicate storage requirement
    2. **Scope Minimally**: Create specific directory for the MCP server's needs
    3. **Update Template**: Add volume to docker-compose.yml.template
    4. **Configure Environment**: Set MCP server environment variables to use the writable path
    5. **Test Security**: Verify container still maintains read-only filesystem outside of mounted volumes

15. **Traefik Integration Modes**:
    - **Local Traefik** (`ENABLE_LOCAL_TRAEFIK=true`): Includes Traefik service in docker-compose, manages SSL certificates, exposes ports 80/443/8080
    - **External Traefik** (`ENABLE_LOCAL_TRAEFIK=false` or omitted): Uses external 'proxy' network, requires existing Traefik setup

16. **Container Health Verification**: Always verify container health before testing external functionality:
    ```bash
    # Step 1: Wait for container to show (healthy) status
    docker-compose ps
    
    # Step 2: Monitor logs for health endpoint activity
    docker logs remote-mcp-proxy --follow
    
    # Step 3: Wait 10 seconds after seeing "Health check response sent successfully"
    # This ensures the health endpoint is fully operational
    
    # Step 4: Verify health endpoint manually
    docker exec remote-mcp-proxy curl -s http://localhost:8080/health
    
    # Step 5: Test external endpoint (only after internal health confirmed)
    curl -s https://yourdomain.com/health
    ```

### Error Handling and Debugging Protocol

#### Dynamic Package System Troubleshooting
14. **Package Detection Issues**:
    - **Dockerfile Generation Fails**: 
      - Verify config.json syntax: `python3 -m json.tool config.json`
      - Check package format: Ensure npx args follow `["-y", "@package/name", ...args]` pattern
      - Debug generation: Run `make generate-dockerfile` and check output for "Found npm package" messages
      
    - **Runtime Conversion Fails**:
      - Check container logs: `docker logs remote-mcp-proxy | head -10` for conversion messages
      - Verify package installation: `docker exec remote-mcp-proxy npm list -g --depth=0`
      - Check binary discovery: `docker exec remote-mcp-proxy ls -la /usr/local/bin/mcp-*`
      - Inspect converted config: `docker exec remote-mcp-proxy cat /tmp/config.json`
      
    - **Unsupported Package Manager**:
      - Currently supported: npx (npm), uvx (Python), python -m (pip)
      - For new managers: Extend `extract_packages_from_config()` in `scripts/generate-dockerfile.py`
      - Add install commands in `generate_install_commands()` function

15. **Systematic Error Resolution**:
    - **Build Failures**: Run `go fmt ./...` and `go vet ./...`, then `go build -o remote-mcp-proxy .`
    - **Container Issues**: Check `docker logs remote-mcp-proxy`, verify health endpoint, inspect network connectivity
    - **Traefik Issues**: Verify `.env` configuration, check certificate status, inspect Traefik dashboard
    - **MCP Server Issues**: Check individual server logs, verify config.json format, test server commands manually

16. **Error Pattern Recognition and Investigation**:
    - **Systematic Analysis**: Use `/investigate` command for complex, multi-component issues
    - **Pattern Identification**: Look for recurring error messages, timing correlations, specific trigger conditions
    - **Evidence Collection**: Gather logs from multiple sources (proxy, containers, external systems)
    - **Root Cause Analysis**: Differentiate between symptoms and underlying causes

#### Systematic Evidence Gathering Protocol
**Concurrent Tool Usage**: Use multiple tools simultaneously for faster diagnosis
```bash
# Example: Parallel evidence gathering for container issues
docker logs container_name --tail=50    # Get recent logs
docker stats container_name --no-stream  # Check resource usage  
docker exec container_name mount | grep ro  # Check read-only mounts
docker-compose ps  # Check service status
```

**Evidence Gathering Checklist**:
1. **Container State**: Status, health, resource usage, process list
2. **Application Logs**: Recent entries, error patterns, debug messages
3. **System Configuration**: Mount points, permissions, environment variables
4. **Network Connectivity**: Service endpoints, internal/external access
5. **File System**: Directory permissions, available space, read/write tests
6. **Package State**: Installed packages, binary locations, version conflicts

**Concurrent Investigation Pattern**:
- Use multiple Bash tool calls in parallel for independent checks
- Batch file reads when examining multiple configuration files
- Run Docker MCP tools alongside system commands for comprehensive analysis
- Combine log analysis with configuration verification in single workflow

    - **Common Patterns**:
      - **Stdio Deadlocks**: `bufio.Scanner.Scan()` blocking, "context deadline exceeded", "file already closed"
      - **Process Hangs**: Unresponsive servers, timeout cascades, resource exhaustion
      - **Network Issues**: Connection refused, SSL certificate problems, DNS resolution failures
      - **Configuration Problems**: Invalid JSON, missing environment variables, permission issues

#### MCP Server Error Patterns
**Storage Permission Errors**:
- `EROFS: read-only file system` - MCP server trying to write to read-only filesystem
- `EACCES: permission denied` - Insufficient write permissions for data directories
- `ENOENT: no such file or directory` - Missing writable volume mounts

**Binary Discovery Failures**:
- `command not found` - MCP server binary not pre-installed or not in PATH
- `npm install` timeouts - Package installation delays causing MCP initialization failures
- Binary path mismatches between pre-installation and runtime conversion

**Protocol Compliance Issues**:
- `Method not found` (code: -32601) - MCP server doesn't implement expected JSON-RPC methods
- `Invalid params` (code: -32602) - Parameter mismatch between proxy and MCP server
- `Internal error` (code: -32603) - MCP server internal failures (often storage-related)

**Environment Configuration Issues**:
- Environment variables not properly passed to converted binary commands
- Path environment variables missing for MCP server dependencies
- Configuration file paths not accessible from container runtime environment

**Performance/Timeout Patterns**:
- Health check failures during MCP server startup (first-time package installation)
- Connection timeouts during npx package resolution
- Memory/resource exhaustion from multiple concurrent MCP server startups

17. **Multi-Layer Fix Implementation Strategy**:
    - **Assessment Phase**: Identify all affected system layers (application, stdio, process management, network)
    - **Coordinated Planning**: Use TodoWrite to plan fixes across multiple layers simultaneously
    - **Implementation Order**: Fix foundational issues first (stdio, process management), then higher-level issues
    - **Testing Strategy**: Test each layer individually, then test integrated functionality
    - **Examples**:
      - **Stdio + Process Management**: Fix buffered I/O handling AND implement process restart mechanisms
      - **Network + Application**: Fix Traefik configuration AND update application health checks
      - **Configuration + Security**: Update environment variables AND certificate management

18. **When Lint/Typecheck Fails**:
    - **Fix Go formatting**: `go fmt ./...` (auto-fixes formatting issues)
    - **Address vet warnings**: Review `go vet ./...` output and fix reported issues
    - **Build verification**: Ensure `go build -o remote-mcp-proxy .` succeeds before deployment
    - **Dependencies**: Run `go mod tidy` if module issues occur

### Testing and Integration Rules
19. **Claude.ai Integration Testing**: If you need the user to test Claude.ai integration, just ask them directly
20. **Use Real URLs for Testing**: Always use the real domain URLs (e.g., `https://memory.mcp.home.pezzos.com/sse`) instead of localhost when testing the complete flow through Traefik
21. **Container Startup Timing**: Remember that the container takes time for its first healthcheck to pass - before the healthcheck succeeds, Traefik won't expose the service. Wait for healthy status before testing external URLs
22. **Multiple Integration Support**: ✅ **FIXED** - The proxy now supports multiple simultaneous Claude.ai integrations without tool interference. Each integration maintains isolated tool discovery and execution.
23. **Connection Cleanup**: The proxy automatically detects client disconnections within 30 seconds using keep-alive messages, with fallback cleanup after 2 minutes for stale connections.

### Connection Management
24. **Manual Cleanup**: Use `docker exec remote-mcp-proxy curl -X POST http://localhost:8080/cleanup` to force immediate cleanup of stale connections if needed
25. **Connection Monitoring**: Check active connections with `docker logs remote-mcp-proxy` - no more continuous "SSE connection active" spam messages
26. **Per-Server Request Queuing**: Requests to the same MCP server are serialized to prevent response mismatching, while different servers process requests concurrently

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

# important-instruction-reminders
Do what has been asked; nothing more, nothing less.
NEVER create files unless they're absolutely necessary for achieving your goal.
ALWAYS prefer editing an existing file to creating a new one.
NEVER proactively create documentation files (*.md) or README files. Only create documentation files if explicitly requested by the User.

## Implementation Behavior Protocol
When user says "implement" or uses `/investigate` command:
- **ALWAYS implement changes directly into relevant config files** (CLAUDE.md, settings files, etc.)
- **NEVER just output instructions** - actually modify the files
- **Use MultiEdit or Edit tools** to make the changes permanent
- **Verify changes are applied** by reading the modified files
- **Update behavior immediately** based on the new instructions