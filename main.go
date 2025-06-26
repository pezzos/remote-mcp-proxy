package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"remote-mcp-proxy/config"
	"remote-mcp-proxy/logger"
	"remote-mcp-proxy/mcp"
	"remote-mcp-proxy/proxy"
)

func main() {
	// Initialize logger system
	loggerManager := logger.GetManager()
	defer loggerManager.Close()

	sysLog := logger.System()
	sysLog.Info("Starting Remote MCP Proxy...")

	// Load configuration
	configPath := os.Getenv("CONFIG_FILE")
	if configPath == "" {
		configPath = "/app/config.json"
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		sysLog.Error("Failed to load configuration: %v", err)
		os.Exit(1)
	}

	// Create MCP manager
	mcpManager := mcp.NewManager(cfg.MCPServers)

	// Start MCP servers
	if err := mcpManager.StartAll(); err != nil {
		sysLog.Error("Failed to start MCP servers: %v", err)
		os.Exit(1)
	}

	// Create proxy server with configuration
	proxyServer := proxy.NewServerWithConfig(mcpManager, cfg)

	// Start HTTP server on configured port
	addr := ":" + cfg.GetPort()
	server := &http.Server{
		Addr:    addr,
		Handler: proxyServer.Router(),
	}

	// Start server in goroutine
	go func() {
		sysLog.Info("Server starting on %s (Domain: %s)", addr, cfg.GetDomain())
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			sysLog.Error("Server failed: %v", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	sysLog.Info("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		sysLog.Warn("Server forced to shutdown: %v", err)
	}

	// Stop MCP servers
	mcpManager.StopAll()

	sysLog.Info("Server exited")
}
