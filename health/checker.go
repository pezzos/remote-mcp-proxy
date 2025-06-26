package health

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"remote-mcp-proxy/logger"
	"remote-mcp-proxy/mcp"
)

type ServerHealth struct {
	Name             string    `json:"name"`
	Status           string    `json:"status"` // healthy, unhealthy, unknown
	LastCheck        time.Time `json:"lastCheck"`
	ResponseTime     int64     `json:"responseTimeMs"`
	ConsecutiveFails int       `json:"consecutiveFails"`
	RestartCount     int       `json:"restartCount"`
	LastError        string    `json:"lastError,omitempty"`
}

type HealthChecker struct {
	mcpManager    *mcp.Manager
	healthStatus  map[string]*ServerHealth
	mu            sync.RWMutex
	checkInterval time.Duration
	maxRestarts   int
	restartWindow time.Duration
	stopChan      chan bool
	logger        *logger.Logger
}

func NewHealthChecker(mcpManager *mcp.Manager) *HealthChecker {
	return &HealthChecker{
		mcpManager:    mcpManager,
		healthStatus:  make(map[string]*ServerHealth),
		checkInterval: 30 * time.Second, // Check every 30 seconds
		maxRestarts:   3,                // Max 3 restarts per window
		restartWindow: 5 * time.Minute,  // 5-minute window
		stopChan:      make(chan bool),
		logger:        logger.System(),
	}
}

func (hc *HealthChecker) Start() {
	hc.logger.Info("Starting MCP server health checker (interval: %v)", hc.checkInterval)

	go func() {
		ticker := time.NewTicker(hc.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				hc.checkAllServers()
			case <-hc.stopChan:
				hc.logger.Info("Health checker stopped")
				return
			}
		}
	}()
}

func (hc *HealthChecker) Stop() {
	close(hc.stopChan)
}

func (hc *HealthChecker) checkAllServers() {
	servers := hc.mcpManager.GetAllServers()

	for _, serverStatus := range servers {
		if !serverStatus.Running {
			hc.updateHealth(serverStatus.Name, "unhealthy", 0, "Server not running")
			continue
		}

		hc.checkServerHealth(serverStatus.Name)
	}
}

func (hc *HealthChecker) checkServerHealth(serverName string) {
	server, exists := hc.mcpManager.GetServer(serverName)
	if !exists {
		hc.updateHealth(serverName, "unknown", 0, "Server not found")
		return
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Send a simple ping message to check responsiveness
	pingMsg := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "ping",
		"id":      "health_check",
	}

	msgBytes, err := json.Marshal(pingMsg)
	if err != nil {
		hc.updateHealth(serverName, "unhealthy", 0, fmt.Sprintf("Failed to marshal ping: %v", err))
		return
	}

	// Try to send ping and get response
	_, err = server.SendAndReceive(ctx, msgBytes)
	responseTime := time.Since(start).Milliseconds()

	if err != nil {
		hc.handleUnhealthyServer(serverName, responseTime, err.Error())
	} else {
		hc.updateHealth(serverName, "healthy", responseTime, "")
	}
}

func (hc *HealthChecker) handleUnhealthyServer(serverName string, responseTime int64, errorMsg string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	health := hc.getOrCreateHealth(serverName)
	health.ConsecutiveFails++
	health.Status = "unhealthy"
	health.LastCheck = time.Now()
	health.ResponseTime = responseTime
	health.LastError = errorMsg

	hc.logger.Warn("Health check failed for server %s (consecutive fails: %d): %s",
		serverName, health.ConsecutiveFails, errorMsg)

	// Check if we should restart the server
	if health.ConsecutiveFails >= 3 && hc.shouldRestartServer(serverName) {
		hc.restartUnhealthyServer(serverName)
	}
}

func (hc *HealthChecker) shouldRestartServer(serverName string) bool {
	health := hc.getOrCreateHealth(serverName)

	// Check if we're within restart limits
	now := time.Now()
	if now.Sub(health.LastCheck) < hc.restartWindow && health.RestartCount >= hc.maxRestarts {
		hc.logger.Warn("Server %s hit restart limit (%d restarts in %v), skipping restart",
			serverName, hc.maxRestarts, hc.restartWindow)
		return false
	}

	// Reset restart count if outside window
	if now.Sub(health.LastCheck) >= hc.restartWindow {
		health.RestartCount = 0
	}

	return true
}

func (hc *HealthChecker) restartUnhealthyServer(serverName string) {
	hc.logger.Warn("Attempting to restart unhealthy server: %s", serverName)

	err := hc.mcpManager.RestartServer(serverName)

	health := hc.getOrCreateHealth(serverName)
	health.RestartCount++

	if err != nil {
		hc.logger.Error("Failed to restart server %s: %v", serverName, err)
		health.LastError = fmt.Sprintf("Restart failed: %v", err)
	} else {
		hc.logger.Info("Successfully restarted server %s (restart count: %d)",
			serverName, health.RestartCount)
		health.ConsecutiveFails = 0
		health.Status = "unknown" // Will be checked on next cycle
		health.LastError = ""
	}
}

func (hc *HealthChecker) updateHealth(serverName, status string, responseTime int64, errorMsg string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	health := hc.getOrCreateHealth(serverName)
	health.Status = status
	health.LastCheck = time.Now()
	health.ResponseTime = responseTime
	health.LastError = errorMsg

	if status == "healthy" {
		health.ConsecutiveFails = 0
	}
}

func (hc *HealthChecker) getOrCreateHealth(serverName string) *ServerHealth {
	if health, exists := hc.healthStatus[serverName]; exists {
		return health
	}

	health := &ServerHealth{
		Name:             serverName,
		Status:           "unknown",
		LastCheck:        time.Now(),
		ConsecutiveFails: 0,
		RestartCount:     0,
	}
	hc.healthStatus[serverName] = health
	return health
}

func (hc *HealthChecker) GetHealthStatus() map[string]*ServerHealth {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	// Return a copy to avoid race conditions
	result := make(map[string]*ServerHealth)
	for name, health := range hc.healthStatus {
		healthCopy := *health
		result[name] = &healthCopy
	}

	return result
}

func (hc *HealthChecker) GetServerHealth(serverName string) (*ServerHealth, bool) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	health, exists := hc.healthStatus[serverName]
	if !exists {
		return nil, false
	}

	// Return a copy
	healthCopy := *health
	return &healthCopy, true
}
