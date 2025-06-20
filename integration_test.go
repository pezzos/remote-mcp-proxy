package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"remote-mcp-proxy/config"
	"remote-mcp-proxy/mcp"
	"remote-mcp-proxy/protocol"
	"remote-mcp-proxy/proxy"
)

func TestMain(m *testing.M) {
	// Setup
	fmt.Println("Setting up integration tests...")

	// Run tests
	code := m.Run()

	// Cleanup
	fmt.Println("Cleaning up integration tests...")

	os.Exit(code)
}

func TestFullMCPProxyWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create test configuration with a simple echo server
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	testConfig := config.Config{
		MCPServers: map[string]config.MCPServer{
			"echo-server": {
				Command: "echo",
				Args:    []string{`{"jsonrpc":"2.0","result":{"status":"ready"}}`},
				Env:     map[string]string{},
			},
		},
	}

	configData, err := json.Marshal(testConfig)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	err = os.WriteFile(configPath, configData, 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load configuration: %v", err)
	}

	// Create and start MCP manager
	mcpManager := mcp.NewManager(cfg.MCPServers)

	// Note: Starting actual processes in tests can be problematic
	// In a real integration test, you might want to start actual MCP servers
	// For now, we'll test the HTTP interface without starting the processes

	// Create proxy server
	proxyServer := proxy.NewServer(mcpManager)
	router := proxyServer.Router()

	// Test health endpoint
	t.Run("health_check", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/health", nil)
		if err != nil {
			t.Fatalf("Failed to create health request: %v", err)
		}

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Health check failed with status %d", rr.Code)
		}

		var healthResponse map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &healthResponse)
		if err != nil {
			t.Errorf("Failed to parse health response: %v", err)
		}

		if status, ok := healthResponse["status"]; !ok || status != "healthy" {
			t.Errorf("Expected healthy status, got %v", healthResponse)
		}
	})

	// Test MCP handshake workflow
	t.Run("mcp_handshake", func(t *testing.T) {
		sessionID := "test-session-123"

		// Step 1: Send initialize request
		initParams := protocol.InitializeParams{
			ProtocolVersion: protocol.MCPProtocolVersion,
			Capabilities:    map[string]interface{}{},
			ClientInfo: protocol.ClientInfo{
				Name:    "test-client",
				Version: "1.0.0",
			},
		}

		initMessage := protocol.JSONRPCMessage{
			JSONRPC: "2.0",
			ID:      "init-1",
			Method:  "initialize",
			Params:  initParams,
		}

		msgBytes, err := json.Marshal(initMessage)
		if err != nil {
			t.Fatalf("Failed to marshal initialize message: %v", err)
		}

		req, err := http.NewRequest("POST", "/echo-server/sse", bytes.NewReader(msgBytes))
		if err != nil {
			t.Fatalf("Failed to create initialize request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Session-ID", sessionID)

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		// The server might not exist (since we didn't start actual processes)
		// but we should get a proper HTTP response
		if rr.Code != http.StatusNotFound && rr.Code != http.StatusOK && rr.Code != http.StatusAccepted {
			t.Logf("Initialize request status: %d, body: %s", rr.Code, rr.Body.String())
		}

		// Step 2: Send initialized notification
		initNotification := protocol.JSONRPCMessage{
			JSONRPC: "2.0",
			Method:  "notifications/initialized",
		}

		notifBytes, err := json.Marshal(initNotification)
		if err != nil {
			t.Fatalf("Failed to marshal initialized notification: %v", err)
		}

		req2, err := http.NewRequest("POST", "/echo-server/sse", bytes.NewReader(notifBytes))
		if err != nil {
			t.Fatalf("Failed to create initialized request: %v", err)
		}
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("X-Session-ID", sessionID)

		rr2 := httptest.NewRecorder()
		router.ServeHTTP(rr2, req2)

		if rr2.Code != http.StatusNotFound && rr2.Code != http.StatusOK && rr2.Code != http.StatusAccepted {
			t.Logf("Initialized notification status: %d, body: %s", rr2.Code, rr2.Body.String())
		}
	})

	// Test SSE connection establishment
	t.Run("sse_connection", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/echo-server/sse", nil)
		if err != nil {
			t.Fatalf("Failed to create SSE request: %v", err)
		}
		req.Header.Set("Accept", "text/event-stream")

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		// Should get Not Found since the server doesn't exist, but headers should be correct
		if rr.Code == http.StatusOK {
			// Check SSE headers
			if contentType := rr.Header().Get("Content-Type"); contentType != "text/event-stream" {
				t.Errorf("Expected Content-Type text/event-stream, got %s", contentType)
			}

			if cacheControl := rr.Header().Get("Cache-Control"); cacheControl != "no-cache" {
				t.Errorf("Expected Cache-Control no-cache, got %s", cacheControl)
			}

			if connection := rr.Header().Get("Connection"); connection != "keep-alive" {
				t.Errorf("Expected Connection keep-alive, got %s", connection)
			}
		}
	})

	// Test CORS headers
	t.Run("cors_headers", func(t *testing.T) {
		req, err := http.NewRequest("OPTIONS", "/health", nil)
		if err != nil {
			t.Fatalf("Failed to create OPTIONS request: %v", err)
		}
		req.Header.Set("Origin", "https://claude.ai")

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("OPTIONS request failed with status %d", rr.Code)
		}

		if origin := rr.Header().Get("Access-Control-Allow-Origin"); origin != "https://claude.ai" {
			t.Errorf("Expected CORS origin https://claude.ai, got %s", origin)
		}

		if methods := rr.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(methods, "GET") {
			t.Errorf("Expected CORS methods to include GET, got %s", methods)
		}
	})

	// Test concurrent connections
	t.Run("concurrent_connections", func(t *testing.T) {
		const numConnections = 10

		// Create multiple concurrent requests
		results := make(chan int, numConnections)

		for i := 0; i < numConnections; i++ {
			go func(id int) {
				req, err := http.NewRequest("GET", "/health", nil)
				if err != nil {
					results <- 500
					return
				}

				rr := httptest.NewRecorder()
				router.ServeHTTP(rr, req)
				results <- rr.Code
			}(i)
		}

		// Collect results
		successCount := 0
		for i := 0; i < numConnections; i++ {
			select {
			case status := <-results:
				if status == http.StatusOK {
					successCount++
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Timeout waiting for concurrent requests")
			}
		}

		if successCount != numConnections {
			t.Errorf("Expected %d successful requests, got %d", numConnections, successCount)
		}
	})
}

func TestProtocolTranslationIntegration(t *testing.T) {
	translator := protocol.NewTranslator()

	// Test round-trip translation
	t.Run("round_trip_translation", func(t *testing.T) {
		// Create a Remote MCP message
		originalRemoteMsg := protocol.RemoteMCPMessage{
			Type:   "request",
			ID:     "test-456",
			Method: "tools/list",
			Params: map[string]interface{}{
				"cursor": "page1",
				"limit":  50,
			},
		}

		// Convert to JSON
		remoteMsgBytes, err := json.Marshal(originalRemoteMsg)
		if err != nil {
			t.Fatalf("Failed to marshal Remote MCP message: %v", err)
		}

		// Translate Remote MCP -> JSON-RPC
		jsonrpcBytes, err := translator.RemoteToMCP(remoteMsgBytes)
		if err != nil {
			t.Fatalf("Failed to translate Remote MCP to JSON-RPC: %v", err)
		}

		// Parse JSON-RPC message
		var jsonrpcMsg protocol.JSONRPCMessage
		err = json.Unmarshal(jsonrpcBytes, &jsonrpcMsg)
		if err != nil {
			t.Fatalf("Failed to unmarshal JSON-RPC message: %v", err)
		}

		// Verify JSON-RPC message
		if jsonrpcMsg.JSONRPC != "2.0" {
			t.Errorf("Expected JSONRPC 2.0, got %s", jsonrpcMsg.JSONRPC)
		}
		if jsonrpcMsg.ID != originalRemoteMsg.ID {
			t.Errorf("Expected ID %v, got %v", originalRemoteMsg.ID, jsonrpcMsg.ID)
		}
		if jsonrpcMsg.Method != originalRemoteMsg.Method {
			t.Errorf("Expected method %s, got %s", originalRemoteMsg.Method, jsonrpcMsg.Method)
		}

		// Translate back JSON-RPC -> Remote MCP
		backToRemoteBytes, err := translator.MCPToRemote(jsonrpcBytes)
		if err != nil {
			t.Fatalf("Failed to translate JSON-RPC to Remote MCP: %v", err)
		}

		// Parse back to Remote MCP
		var backToRemoteMsg protocol.RemoteMCPMessage
		err = json.Unmarshal(backToRemoteBytes, &backToRemoteMsg)
		if err != nil {
			t.Fatalf("Failed to unmarshal back to Remote MCP: %v", err)
		}

		// Verify round-trip
		if backToRemoteMsg.Type != originalRemoteMsg.Type {
			t.Errorf("Round-trip failed: type %s != %s", backToRemoteMsg.Type, originalRemoteMsg.Type)
		}
		if backToRemoteMsg.ID != originalRemoteMsg.ID {
			t.Errorf("Round-trip failed: ID %v != %v", backToRemoteMsg.ID, originalRemoteMsg.ID)
		}
		if backToRemoteMsg.Method != originalRemoteMsg.Method {
			t.Errorf("Round-trip failed: method %s != %s", backToRemoteMsg.Method, originalRemoteMsg.Method)
		}
	})

	// Test handshake workflow
	t.Run("handshake_workflow", func(t *testing.T) {
		sessionID := "integration-test-session"

		// Initialize
		initParams := protocol.InitializeParams{
			ProtocolVersion: protocol.MCPProtocolVersion,
			Capabilities: map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			ClientInfo: protocol.ClientInfo{
				Name:    "integration-test-client",
				Version: "1.0.0",
			},
		}

		result, err := translator.HandleInitialize(sessionID, initParams)
		if err != nil {
			t.Fatalf("Initialize failed: %v", err)
		}

		if result.ProtocolVersion != protocol.MCPProtocolVersion {
			t.Errorf("Expected protocol version %s, got %s", protocol.MCPProtocolVersion, result.ProtocolVersion)
		}

		if result.ServerInfo.Name != protocol.ProxyServerName {
			t.Errorf("Expected server name %s, got %s", protocol.ProxyServerName, result.ServerInfo.Name)
		}

		// Check that session is not yet initialized
		if translator.IsInitialized(sessionID) {
			t.Error("Session should not be initialized before initialized notification")
		}

		// Send initialized notification
		err = translator.HandleInitialized(sessionID)
		if err != nil {
			t.Fatalf("HandleInitialized failed: %v", err)
		}

		// Check that session is now initialized
		if !translator.IsInitialized(sessionID) {
			t.Error("Session should be initialized after initialized notification")
		}

		// Verify connection state
		state, exists := translator.GetConnectionState(sessionID)
		if !exists {
			t.Error("Expected connection state to exist")
		}

		if state.SessionID != sessionID {
			t.Errorf("Expected session ID %s, got %s", sessionID, state.SessionID)
		}

		if state.ProtocolVersion != protocol.MCPProtocolVersion {
			t.Errorf("Expected protocol version %s, got %s", protocol.MCPProtocolVersion, state.ProtocolVersion)
		}

		// Clean up
		translator.RemoveConnection(sessionID)

		// Verify cleanup
		if translator.IsInitialized(sessionID) {
			t.Error("Session should not be initialized after removal")
		}

		_, exists = translator.GetConnectionState(sessionID)
		if exists {
			t.Error("Connection state should not exist after removal")
		}
	})
}

func TestErrorHandlingIntegration(t *testing.T) {
	// Create proxy with no servers
	mcpManager := mcp.NewManager(map[string]config.MCPServer{})
	proxyServer := proxy.NewServer(mcpManager)
	router := proxyServer.Router()

	// Test accessing non-existent server
	t.Run("non_existent_server", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/non-existent-server/sse", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", rr.Code)
		}

		if !strings.Contains(rr.Body.String(), "not found") {
			t.Errorf("Expected error message about server not found, got: %s", rr.Body.String())
		}
	})

	// Test malformed JSON
	t.Run("malformed_json", func(t *testing.T) {
		// First, add a server to test with
		configs := map[string]config.MCPServer{
			"test-server": {
				Command: "echo",
				Args:    []string{"test"},
			},
		}
		mcpManager := mcp.NewManager(configs)
		proxyServer := proxy.NewServer(mcpManager)
		router := proxyServer.Router()

		malformedJSON := `{"jsonrpc":"2.0","method":"test",`
		req, err := http.NewRequest("POST", "/test-server/sse", strings.NewReader(malformedJSON))
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", rr.Code)
		}
	})

	// Test invalid method
	t.Run("invalid_method", func(t *testing.T) {
		req, err := http.NewRequest("PUT", "/health", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status 405, got %d", rr.Code)
		}
	})
}

func TestConnectionLimitIntegration(t *testing.T) {
	mcpManager := mcp.NewManager(map[string]config.MCPServer{})
	_ = proxy.NewServer(mcpManager) // Create but don't use directly

	// Test connection manager limits
	t.Run("connection_limits", func(t *testing.T) {
		cm := proxy.NewConnectionManager(2) // Very low limit for testing

		ctx1, cancel1 := context.WithCancel(context.Background())
		ctx2, cancel2 := context.WithCancel(context.Background())
		ctx3, cancel3 := context.WithCancel(context.Background())

		defer cancel1()
		defer cancel2()
		defer cancel3()

		// Add connections up to limit
		err1 := cm.AddConnection("session-1", "server-1", ctx1, cancel1)
		if err1 != nil {
			t.Errorf("Unexpected error adding connection 1: %v", err1)
		}

		err2 := cm.AddConnection("session-2", "server-2", ctx2, cancel2)
		if err2 != nil {
			t.Errorf("Unexpected error adding connection 2: %v", err2)
		}

		// This should fail
		err3 := cm.AddConnection("session-3", "server-3", ctx3, cancel3)
		if err3 == nil {
			t.Error("Expected error when exceeding connection limit")
		}

		// Remove one and try again
		cm.RemoveConnection("session-1")

		err4 := cm.AddConnection("session-3", "server-3", ctx3, cancel3)
		if err4 != nil {
			t.Errorf("Unexpected error after removing connection: %v", err4)
		}

		// Verify final state
		if count := cm.GetConnectionCount(); count != 2 {
			t.Errorf("Expected 2 connections, got %d", count)
		}
	})
}

// Helper function to create a test server with real HTTP listener
func createTestServer(t *testing.T) (*httptest.Server, *mcp.Manager) {
	configs := map[string]config.MCPServer{
		"test-server": {
			Command: "echo",
			Args:    []string{"test"},
		},
	}

	mcpManager := mcp.NewManager(configs)
	proxyServer := proxy.NewServer(mcpManager)

	server := httptest.NewServer(proxyServer.Router())
	return server, mcpManager
}

func TestRealHTTPIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real HTTP test in short mode")
	}

	server, _ := createTestServer(t)
	defer server.Close()

	// Test health endpoint over real HTTP
	t.Run("real_http_health", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/health")
		if err != nil {
			t.Fatalf("Failed to make health request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Failed to read response body: %v", err)
		}

		var healthResponse map[string]interface{}
		err = json.Unmarshal(body, &healthResponse)
		if err != nil {
			t.Errorf("Failed to parse health response: %v", err)
		}

		if status, ok := healthResponse["status"]; !ok || status != "healthy" {
			t.Errorf("Expected healthy status, got %v", healthResponse)
		}
	})

	// Test CORS with real HTTP
	t.Run("real_http_cors", func(t *testing.T) {
		client := &http.Client{}
		req, err := http.NewRequest("OPTIONS", server.URL+"/health", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}
		req.Header.Set("Origin", "https://claude.ai")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to make CORS request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}

		if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "https://claude.ai" {
			t.Errorf("Expected CORS origin https://claude.ai, got %s", origin)
		}
	})
}
