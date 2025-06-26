# Monitoring and Health Management Guide

This document provides comprehensive guidance on monitoring, health management, and resource tracking for the Remote MCP Proxy.

## Overview

The Remote MCP Proxy includes sophisticated monitoring capabilities designed to ensure reliable operation and prevent common issues like server hangs, resource exhaustion, and service degradation.

## 🏥 Health Monitoring System

### Health Check Architecture

The proxy implements a multi-layer health monitoring system:

1. **System Health**: Overall proxy service health
2. **MCP Server Health**: Individual server health with automatic recovery
3. **Resource Monitoring**: Process-level resource tracking and alerting
4. **Connection Management**: Active connection tracking and cleanup

### Health Checker Configuration

```go
// Default health checker settings
checkInterval: 30 * time.Second  // Check every 30 seconds
maxRestarts:   3                 // Max 3 restarts per window
restartWindow: 5 * time.Minute   // 5-minute restart window
```

### Health Status Levels

- **healthy**: Server responding normally
- **unhealthy**: Server not responding, recovery in progress
- **unknown**: Server status not yet determined

## 📊 Monitoring Endpoints

### 1. System Health Endpoint

**Endpoint**: `GET /health`

```bash
curl https://mcp.your-domain.com/health
```

**Response**:
```json
{
  "status": "healthy"
}
```

**Use Cases**:
- Load balancer health checks
- Uptime monitoring
- Basic service availability

### 2. Detailed Server Health

**Endpoint**: `GET /health/servers`

```bash
curl https://mcp.your-domain.com/health/servers
```

**Response**:
```json
{
  "timestamp": "2025-06-26T10:30:00Z",
  "servers": {
    "memory": {
      "name": "memory",
      "status": "healthy",
      "lastCheck": "2025-06-26T10:29:45Z",
      "responseTimeMs": 120,
      "consecutiveFails": 0,
      "restartCount": 0,
      "lastError": ""
    },
    "sequential-thinking": {
      "name": "sequential-thinking", 
      "status": "unhealthy",
      "lastCheck": "2025-06-26T10:29:30Z",
      "responseTimeMs": 30000,
      "consecutiveFails": 2,
      "restartCount": 1,
      "lastError": "context deadline exceeded"
    }
  },
  "summary": {
    "total": 4,
    "healthy": 3,
    "unhealthy": 1,
    "unknown": 0
  }
}
```

**Response Fields**:
- `responseTimeMs`: Response time for health check (milliseconds)
- `consecutiveFails`: Number of consecutive failed health checks
- `restartCount`: Number of restarts in current window
- `lastError`: Most recent error message

**Use Cases**:
- Detailed health monitoring
- Performance tracking
- Restart pattern analysis
- Alerting and notifications

### 3. Resource Metrics

**Endpoint**: `GET /health/resources`

```bash
curl https://mcp.your-domain.com/health/resources
```

**Response**:
```json
{
  "timestamp": "2025-06-26T10:30:00Z",
  "processes": [
    {
      "pid": 123,
      "name": "memory-server",
      "memoryMB": 145.2,
      "cpuPercent": 2.1,
      "virtualMB": 512.0,
      "residentMB": 145.2,
      "timestamp": "2025-06-26T10:30:00Z"
    },
    {
      "pid": 124,
      "name": "sequential-thinking-server",
      "memoryMB": 89.3,
      "cpuPercent": 1.5,
      "virtualMB": 256.0,
      "residentMB": 89.3,
      "timestamp": "2025-06-26T10:30:00Z"
    }
  ],
  "summary": {
    "processCount": 4,
    "totalMemoryMB": 580.5,
    "totalCPU": 8.3,
    "averageMemoryMB": 145.1,
    "averageCPU": 2.1
  }
}
```

**Response Fields**:
- `memoryMB`: Actual memory usage (calculated from system memory percentage)
- `cpuPercent`: CPU usage percentage
- `virtualMB`: Virtual memory size (VSZ)
- `residentMB`: Resident memory size (RSS)

**Use Cases**:
- Resource planning and capacity management
- Performance optimization
- Memory leak detection
- Resource exhaustion prevention

## 🚨 Automatic Recovery System

### Health Check Process

1. **Periodic Ping**: Every 30 seconds, send ping to each MCP server
2. **Failure Detection**: Track consecutive failed health checks
3. **Recovery Trigger**: After 3 consecutive failures, initiate recovery
4. **Smart Restart**: Graceful server restart with process cleanup
5. **Restart Limits**: Maximum 3 restarts per 5-minute window

### Recovery Process

```
Health Check Failure → Consecutive Fail Count++ → 3 Failures?
                                                      ↓ Yes
Check Restart Limits → Within Limits? → Restart Server → Reset Counters
                          ↓ No
                    Log Warning + Skip Restart
```

### Restart Limit Logic

```go
// Reset restart count if outside 5-minute window
if now.Sub(health.LastCheck) >= 5*time.Minute {
    health.RestartCount = 0
}

// Check restart limits
if health.RestartCount >= 3 {
    // Skip restart, log warning
    return false
}
```

## 📈 Resource Monitoring

### Process Discovery

The resource monitor automatically detects MCP-related processes by scanning for keywords:

```go
mcpKeywords := []string{
    "mcp-server",
    "@modelcontextprotocol",
    "@notionhq/notion-mcp-server",
    "sequential-thinking",
    "server-memory",
    "server-filesystem",
}
```

### Alert Thresholds

Default thresholds trigger warnings in logs:

```go
alertThresholds := map[string]float64{
    "memory_mb":    500,  // Alert if any process uses > 500MB
    "cpu_percent":  80,   // Alert if any process uses > 80% CPU
}
```

### Resource Monitoring Frequency

- **Monitoring Interval**: Every 60 seconds
- **Logging**: Resource summaries logged at INFO level
- **Alerts**: Threshold violations logged at WARN level

## 🗂️ Logging System

### Log File Structure

```
/logs/
├── system.log                    # Proxy system logs
├── mcp-memory.log                # Memory server logs
├── mcp-sequential-thinking.log   # Sequential thinking server logs
├── mcp-notion.log               # Notion server logs
└── mcp-filesystem.log           # Filesystem server logs
```

### Log Levels

| Level | System Default | MCP Default | Description |
|-------|----------------|-------------|-------------|
| TRACE | ❌ | ❌ | Detailed execution traces |
| DEBUG | ❌ | ✅ | Debug information |
| INFO | ✅ | ✅ | General information |
| WARN | ✅ | ✅ | Warning conditions |
| ERROR | ✅ | ✅ | Error conditions |

### Log Configuration

Environment variables in `.env`:

```bash
# Log levels
LOG_LEVEL_SYSTEM=INFO      # System-wide logging level
LOG_LEVEL_MCP=DEBUG        # MCP server logging level

# Log retention
LOG_RETENTION_SYSTEM=24h   # System log retention period
LOG_RETENTION_MCP=12h      # MCP log retention period
```

### Log Format

```
2025/06/26 10:30:15.123456 [INFO] Health check passed for server memory (response: 120ms)
2025/06/26 10:30:16.789012 [WARN] High memory usage for process memory-server (PID 123): 520.1MB
2025/06/26 10:30:17.345678 [ERROR] Health check failed for server sequential-thinking: context deadline exceeded
```

### Enhanced Request Tracing

All requests include correlation IDs for better debugging:

```
2025/06/26 10:30:15 [INFO] Method: initialize, ID: 0, SessionID: abc123-def456
2025/06/26 10:30:15 [DEBUG] === MCP SEND AND RECEIVE START (Server: memory) ===
2025/06/26 10:30:15 [INFO] Successfully received response from server memory
2025/06/26 10:30:15 [DEBUG] === MCP SEND AND RECEIVE END (Server: memory) - SUCCESS ===
```

## 🔧 Container Resource Management

### Resource Limits

Docker Compose configuration with resource management:

```yaml
services:
  remote-mcp-proxy:
    deploy:
      resources:
        limits:
          memory: 2G        # Hard memory limit
          cpus: '2.0'       # Hard CPU limit (2 cores)
        reservations:
          memory: 512M      # Guaranteed memory
          cpus: '0.5'       # Guaranteed CPU (0.5 cores)
```

### Benefits of Resource Limits

1. **OOM Prevention**: Hard memory limits prevent system-wide memory exhaustion
2. **CPU Protection**: CPU limits prevent one container from monopolizing CPU
3. **Predictable Performance**: Reservations ensure baseline resource availability
4. **Container Stability**: Improved overall system reliability

### Resource Limit Monitoring

Monitor resource usage relative to limits:

```bash
# Check container resource usage
docker stats remote-mcp-proxy

# Example output:
# CONTAINER ID   NAME               CPU %     MEM USAGE / LIMIT     MEM %
# abc123def456   remote-mcp-proxy   15.2%     1.2GiB / 2.0GiB      60.0%
```

## 📱 External Monitoring Integration

### Prometheus Integration

Example Prometheus configuration:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'mcp-proxy-health'
    static_configs:
      - targets: ['mcp.your-domain.com']
    metrics_path: '/health/servers'
    scheme: https
    scrape_interval: 30s
    
  - job_name: 'mcp-proxy-resources'
    static_configs:
      - targets: ['mcp.your-domain.com']
    metrics_path: '/health/resources'
    scheme: https
    scrape_interval: 60s
```

### Grafana Dashboard

Key metrics to track:

1. **Server Health Status**: Number of healthy/unhealthy servers
2. **Response Times**: Health check response times over time
3. **Restart Frequency**: Server restart patterns
4. **Resource Usage**: Memory and CPU usage trends
5. **Process Count**: Number of active MCP processes

### Alert Rules

Example alerting rules:

```yaml
# Prometheus alert rules
groups:
  - name: mcp-proxy-alerts
    rules:
      - alert: MCPServerUnhealthy
        expr: mcp_server_health_status{status="unhealthy"} > 0
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "MCP server {{ $labels.server }} is unhealthy"
          
      - alert: MCPHighMemoryUsage
        expr: mcp_process_memory_mb > 500
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "MCP process {{ $labels.name }} using {{ $value }}MB memory"
          
      - alert: MCPFrequentRestarts
        expr: increase(mcp_server_restarts[10m]) > 2
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "MCP server {{ $labels.server }} restarting frequently"
```

### Uptime Monitoring

Configure uptime monitoring services:

```bash
# Primary health endpoint
https://mcp.your-domain.com/health

# Expected response: {"status":"healthy"}
# Alert if: Response != 200 OR body != '{"status":"healthy"}'
```

### Log Aggregation

Ship logs to centralized logging:

```yaml
# Docker Compose with log driver
services:
  remote-mcp-proxy:
    logging:
      driver: "json-file"
      options:
        max-size: "100m"
        max-file: "5"
```

Or use log shipping agents like Filebeat, Fluentd, or Promtail to collect logs from the `/logs` volume.

## 🚨 Troubleshooting Monitoring Issues

### Health Checker Not Starting

**Symptoms**: No health check logs, servers not being monitored

**Check**:
```bash
# Verify health checker initialization
docker logs remote-mcp-proxy | grep "Starting MCP server health checker"

# Expected: "Starting MCP server health checker (interval: 30s)"
```

**Resolution**: Ensure proper health checker initialization in main.go

### Resource Monitor Not Working

**Symptoms**: No resource metrics available at `/health/resources`

**Check**:
```bash
# Verify resource monitor initialization
docker logs remote-mcp-proxy | grep "Starting resource monitor"

# Check if ps command works inside container
docker exec remote-mcp-proxy ps aux
```

**Resolution**: Ensure container has proper permissions to run `ps` command

### High Resource Usage

**Symptoms**: Frequent memory/CPU alerts, poor performance

**Investigation**:
```bash
# Check current resource usage
curl https://mcp.your-domain.com/health/resources

# Monitor trends over time
docker stats remote-mcp-proxy

# Check for memory leaks in MCP servers
docker exec remote-mcp-proxy ps aux | grep mcp
```

**Resolution**: 
- Increase container limits if needed
- Investigate specific MCP server resource usage
- Consider process restart limits adjustment

### Frequent Server Restarts

**Symptoms**: Many restart events in logs, unstable service

**Investigation**:
```bash
# Check health status and restart counts
curl https://mcp.your-domain.com/health/servers

# Review MCP server logs for errors
docker exec remote-mcp-proxy tail -f /app/logs/mcp-problematic-server.log
```

**Resolution**:
- Identify root cause of server failures
- Adjust health check timeouts if needed
- Consider MCP server configuration issues

## 📋 Monitoring Checklist

### Daily Monitoring

- [ ] Check overall system health: `/health`
- [ ] Review server health status: `/health/servers`
- [ ] Monitor resource usage trends: `/health/resources`
- [ ] Check log files for errors and warnings
- [ ] Verify all expected MCP servers are running

### Weekly Monitoring

- [ ] Review restart patterns and frequency
- [ ] Analyze resource usage trends over time
- [ ] Check log retention and cleanup
- [ ] Verify monitoring alerts are working
- [ ] Review container resource utilization

### Monthly Monitoring

- [ ] Capacity planning based on resource trends
- [ ] Update alert thresholds if needed
- [ ] Review and optimize MCP server configurations
- [ ] Update monitoring documentation
- [ ] Validate backup and recovery procedures

This comprehensive monitoring system provides the visibility and automation needed to maintain reliable MCP proxy operations in production environments.