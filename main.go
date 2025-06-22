package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"remote-mcp-proxy/config"
	"remote-mcp-proxy/mcp"
	"remote-mcp-proxy/proxy"
)

func main() {
	log.Println("Starting Remote MCP Proxy...")

	// Load configuration
	configPath := os.Getenv("CONFIG_FILE")
	if configPath == "" {
		configPath = "/app/config.json"
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create MCP manager
	mcpManager := mcp.NewManager(cfg.MCPServers)

	// Start MCP servers
	if err := mcpManager.StartAll(); err != nil {
		log.Fatalf("Failed to start MCP servers: %v", err)
	}

	// Create proxy server
	proxyServer := proxy.NewServer(mcpManager)

	// Start HTTP server
	server := &http.Server{
		Addr:    ":8080",
		Handler: proxyServer.Router(),
	}

	// Start server in goroutine
	go func() {
		log.Println("Server starting on :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// Stop MCP servers
	mcpManager.StopAll()

	log.Println("Server exited")
}
