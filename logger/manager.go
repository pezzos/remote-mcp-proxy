package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Manager struct {
	systemLogger    *Logger
	mcpLoggers      map[string]*Logger
	mu              sync.RWMutex
	systemLevel     LogLevel
	mcpLevel        LogLevel
	systemRetention time.Duration
	mcpRetention    time.Duration
}

func NewManager() *Manager {
	return &Manager{
		mcpLoggers: make(map[string]*Logger),
	}
}

func (m *Manager) Initialize() error {
	// Parse environment variables
	systemLevelStr := os.Getenv("LOG_LEVEL_SYSTEM")
	if systemLevelStr == "" {
		systemLevelStr = "INFO"
	}
	m.systemLevel = ParseLogLevel(systemLevelStr)

	mcpLevelStr := os.Getenv("LOG_LEVEL_MCP")
	if mcpLevelStr == "" {
		mcpLevelStr = "DEBUG"
	}
	m.mcpLevel = ParseLogLevel(mcpLevelStr)

	// Parse retention durations
	systemRetentionStr := os.Getenv("LOG_RETENTION_SYSTEM")
	if systemRetentionStr == "" {
		systemRetentionStr = "24h"
	}
	var err error
	m.systemRetention, err = ParseDuration(systemRetentionStr)
	if err != nil {
		return fmt.Errorf("invalid LOG_RETENTION_SYSTEM: %w", err)
	}

	mcpRetentionStr := os.Getenv("LOG_RETENTION_MCP")
	if mcpRetentionStr == "" {
		mcpRetentionStr = "12h"
	}
	m.mcpRetention, err = ParseDuration(mcpRetentionStr)
	if err != nil {
		return fmt.Errorf("invalid LOG_RETENTION_MCP: %w", err)
	}

	// Initialize system logger
	systemConfig := Config{
		Level:     m.systemLevel,
		Filename:  "/app/logs/system.log",
		Retention: m.systemRetention,
	}

	m.systemLogger, err = New(systemConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize system logger: %w", err)
	}

	return nil
}

func (m *Manager) GetSystemLogger() *Logger {
	return m.systemLogger
}

func (m *Manager) GetMCPLogger(serverName string) (*Logger, error) {
	m.mu.RLock()
	logger, exists := m.mcpLoggers[serverName]
	m.mu.RUnlock()

	if exists {
		return logger, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if logger, exists := m.mcpLoggers[serverName]; exists {
		return logger, nil
	}

	// Extract session ID and base server name from server name (format: servername-sessionid)
	sessionID := ""
	baseServerName := serverName
	if parts := strings.Split(serverName, "-"); len(parts) >= 2 {
		// First part is the base server name (e.g., memory, filesystem, etc.)
		baseServerName = parts[0]
		// Join all parts after the first one as session ID (e.g., memory-test-new -> test-new)
		sessionID = strings.Join(parts[1:], "-")
	}

	// Create new MCP logger using ONLY base server name for filename (no session ID)
	filename := filepath.Join("/app/logs", fmt.Sprintf("mcp-%s.log", baseServerName))
	config := Config{
		Level:     m.mcpLevel,
		Filename:  filename,
		Retention: m.mcpRetention,
		SessionID: sessionID,
	}

	logger, err := New(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP logger for %s: %w", serverName, err)
	}

	m.mcpLoggers[serverName] = logger
	return logger, nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var lastErr error

	if m.systemLogger != nil {
		if err := m.systemLogger.Close(); err != nil {
			lastErr = err
		}
	}

	for _, logger := range m.mcpLoggers {
		if err := logger.Close(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// Global logger manager instance
var globalManager *Manager
var initOnce sync.Once

func GetManager() *Manager {
	initOnce.Do(func() {
		globalManager = NewManager()
		if err := globalManager.Initialize(); err != nil {
			// Fallback to stdout logging if initialization fails
			fmt.Printf("Failed to initialize logger manager: %v\n", err)
		}
	})
	return globalManager
}

// Convenience functions for system logging
func System() *Logger {
	return GetManager().GetSystemLogger()
}

func MCP(serverName string) (*Logger, error) {
	return GetManager().GetMCPLogger(serverName)
}
