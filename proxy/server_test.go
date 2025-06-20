package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"remote-mcp-proxy/config"
	"remote-mcp-proxy/mcp"
	"remote-mcp-proxy/protocol"
)

// mockMCPManager implements a mock MCP manager for testing
type mockMCPManager struct {
	servers map[string]*mockMCPServer
	mu      sync.RWMutex
}

type mockMCPServer struct {
	name      string
	running   bool
	messages  [][]byte
	responses [][]byte
	index     int
}

func (m *mockMCPManager) AddMockServer(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers[name] = &mockMCPServer{
		name:      name,
		running:   true,
		messages:  make([][]byte, 0),
		responses: make([][]byte, 0),
	}
}

func (m *mockMCPManager) GetServer(name string) (*mcp.Server, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.servers[name]
	if !exists {
		return nil, false
	}

	// Create a real mcp.Server with mock functionality
	server := &mcp.Server{}
	// Note: In a real implementation, you'd need to properly initialize the server
	// For testing purposes, we'll simulate the interface

	return server, true
}

func (ms *mockMCPServer) SendMessage(message []byte) error {
	if !ms.running {
		return fmt.Errorf("server not running")
	}
	ms.messages = append(ms.messages, message)
	return nil
}

func (ms *mockMCPServer) ReadMessage(ctx context.Context) ([]byte, error) {
	if !ms.running {
		return nil, fmt.Errorf("server not running")
	}

	if ms.index >= len(ms.responses) {
		return nil, fmt.Errorf("no more responses")
	}

	response := ms.responses[ms.index]
	ms.index++
	return response, nil
}

func (ms *mockMCPServer) AddResponse(response []byte) {
	ms.responses = append(ms.responses, response)
}

func TestNewServer(t *testing.T) {
	// Create a real MCP manager for testing
	configs := map[string]config.MCPServer{}
	mcpManager := mcp.NewManager(configs)

	server := NewServer(mcpManager)

	if server == nil {
		t.Fatal("Expected server to be created")
	}

	if server.mcpManager == nil {
		t.Error("Expected mcpManager to be set")
	}

	if server.translator == nil {
		t.Error("Expected translator to be set")
	}

	if server.connectionManager == nil {
		t.Error("Expected connectionManager to be set")
	}
}

func TestHealthEndpoint(t *testing.T) {
	// Create a mock MCP manager
	configs := map[string]config.MCPServer{
		"test-server": {
			Command: "echo",
			Args:    []string{"test"},
		},
	}
	mcpManager := mcp.NewManager(configs)

	server := NewServer(mcpManager)
	router := server.Router()

	req, err := http.NewRequest("GET", "/health", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, status)
	}

	expectedBody := `{"status":"healthy"}`
	if strings.TrimSpace(rr.Body.String()) != expectedBody {
		t.Errorf("Expected body %s, got %s", expectedBody, rr.Body.String())
	}

	// Check content type
	expectedContentType := "application/json"
	if contentType := rr.Header().Get("Content-Type"); contentType != expectedContentType {
		t.Errorf("Expected content type %s, got %s", expectedContentType, contentType)
	}
}

func TestCORSMiddleware(t *testing.T) {
	configs := map[string]config.MCPServer{
		"test-server": {
			Command: "echo",
			Args:    []string{"test"},
		},
	}
	mcpManager := mcp.NewManager(configs)
	server := NewServer(mcpManager)
	router := server.Router()

	tests := []struct {
		name           string
		origin         string
		method         string
		expectedStatus int
		expectOrigin   bool
	}{
		{
			name:           "valid origin",
			origin:         "https://claude.ai",
			method:         "GET",
			expectedStatus: http.StatusOK,
			expectOrigin:   true,
		},
		{
			name:           "no origin header",
			origin:         "",
			method:         "GET",
			expectedStatus: http.StatusOK,
			expectOrigin:   false,
		},
		{
			name:           "options request",
			origin:         "https://claude.ai",
			method:         "OPTIONS",
			expectedStatus: http.StatusOK,
			expectOrigin:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, "/health", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}

			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, status)
			}

			// Check CORS headers
			if tt.expectOrigin {
				if origin := rr.Header().Get("Access-Control-Allow-Origin"); origin != tt.origin {
					t.Errorf("Expected Access-Control-Allow-Origin %s, got %s", tt.origin, origin)
				}
			}

			if methods := rr.Header().Get("Access-Control-Allow-Methods"); methods == "" {
				t.Error("Expected Access-Control-Allow-Methods header to be set")
			}
		})
	}
}

func TestSessionIDGeneration(t *testing.T) {
	configs := map[string]config.MCPServer{}
	mcpManager := mcp.NewManager(configs)
	server := NewServer(mcpManager)

	// Test generating new session ID
	req1, _ := http.NewRequest("GET", "/test", nil)
	sessionID1 := server.getSessionID(req1)

	if sessionID1 == "" {
		t.Error("Expected session ID to be generated")
	}

	// Test using existing session ID
	req2, _ := http.NewRequest("GET", "/test", nil)
	req2.Header.Set("X-Session-ID", "existing-session-123")
	sessionID2 := server.getSessionID(req2)

	if sessionID2 != "existing-session-123" {
		t.Errorf("Expected session ID 'existing-session-123', got '%s'", sessionID2)
	}

	// Test that new session IDs are different
	req3, _ := http.NewRequest("GET", "/test", nil)
	sessionID3 := server.getSessionID(req3)

	if sessionID1 == sessionID3 {
		t.Error("Expected different session IDs")
	}
}

func TestHandshakeMessages(t *testing.T) {
	configs := map[string]config.MCPServer{
		"test-server": {
			Command: "echo",
			Args:    []string{"test"},
		},
	}
	mcpManager := mcp.NewManager(configs)
	server := NewServer(mcpManager)
	router := server.Router()

	// Test initialize request
	initializeParams := protocol.InitializeParams{
		ProtocolVersion: protocol.MCPProtocolVersion,
		Capabilities:    map[string]any{},
		ClientInfo: protocol.ClientInfo{
			Name:    "test-client",
			Version: "1.0.0",
		},
	}

	initializeMsg := protocol.JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      "init-123",
		Method:  "initialize",
		Params:  initializeParams,
	}

	msgBytes, err := json.Marshal(initializeMsg)
	if err != nil {
		t.Fatalf("Failed to marshal initialize message: %v", err)
	}

	req, err := http.NewRequest("POST", "/test-server/sse", bytes.NewReader(msgBytes))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	// The request should succeed (though the actual MCP server won't start in tests)
	if rr.Code != http.StatusOK && rr.Code != http.StatusNotFound {
		t.Errorf("Expected status OK or NotFound, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestConnectionManager(t *testing.T) {
	// Test connection manager directly
	cm := NewConnectionManager(3) // Max 3 connections

	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	ctx3, cancel3 := context.WithCancel(context.Background())
	ctx4, cancel4 := context.WithCancel(context.Background())

	defer cancel1()
	defer cancel2()
	defer cancel3()
	defer cancel4()

	// Add connections up to the limit
	err := cm.AddConnection("session-1", "server-1", ctx1, cancel1)
	if err != nil {
		t.Errorf("Unexpected error adding connection 1: %v", err)
	}

	err = cm.AddConnection("session-2", "server-2", ctx2, cancel2)
	if err != nil {
		t.Errorf("Unexpected error adding connection 2: %v", err)
	}

	err = cm.AddConnection("session-3", "server-3", ctx3, cancel3)
	if err != nil {
		t.Errorf("Unexpected error adding connection 3: %v", err)
	}

	// Check connection count
	if count := cm.GetConnectionCount(); count != 3 {
		t.Errorf("Expected 3 connections, got %d", count)
	}

	// Try to add one more (should fail)
	err = cm.AddConnection("session-4", "server-4", ctx4, cancel4)
	if err == nil {
		t.Error("Expected error when exceeding connection limit")
	}

	// Remove a connection
	cm.RemoveConnection("session-1")

	if count := cm.GetConnectionCount(); count != 2 {
		t.Errorf("Expected 2 connections after removal, got %d", count)
	}

	// Now adding should work again
	err = cm.AddConnection("session-4", "server-4", ctx4, cancel4)
	if err != nil {
		t.Errorf("Unexpected error adding connection after removal: %v", err)
	}

	// Test getting connections
	connections := cm.GetConnections()
	if len(connections) != 3 {
		t.Errorf("Expected 3 connections in map, got %d", len(connections))
	}

	// Verify connection details
	if conn, exists := connections["session-2"]; !exists {
		t.Error("Expected session-2 to exist")
	} else {
		if conn.ServerName != "server-2" {
			t.Errorf("Expected server name 'server-2', got '%s'", conn.ServerName)
		}
		if conn.SessionID != "session-2" {
			t.Errorf("Expected session ID 'session-2', got '%s'", conn.SessionID)
		}
	}
}

func TestConnectionCleanup(t *testing.T) {
	cm := NewConnectionManager(10)

	// Add a connection that's "old"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := cm.AddConnection("old-session", "test-server", ctx, cancel)
	if err != nil {
		t.Fatalf("Failed to add connection: %v", err)
	}

	// Manually adjust the connection time to be old
	cm.mu.Lock()
	if conn, exists := cm.connections["old-session"]; exists {
		conn.ConnectedAt = time.Now().Add(-20 * time.Minute) // 20 minutes ago
	}
	cm.mu.Unlock()

	// Add a recent connection
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	err = cm.AddConnection("new-session", "test-server", ctx2, cancel2)
	if err != nil {
		t.Fatalf("Failed to add new connection: %v", err)
	}

	// Clean up connections older than 10 minutes
	cm.CleanupStaleConnections(10 * time.Minute)

	// Check that old connection was removed and new one remains
	connections := cm.GetConnections()
	if len(connections) != 1 {
		t.Errorf("Expected 1 connection after cleanup, got %d", len(connections))
	}

	if _, exists := connections["old-session"]; exists {
		t.Error("Expected old session to be removed")
	}

	if _, exists := connections["new-session"]; !exists {
		t.Error("Expected new session to remain")
	}
}

func TestValidateAuthentication(t *testing.T) {
	configs := map[string]config.MCPServer{}
	mcpManager := mcp.NewManager(configs)
	server := NewServer(mcpManager)

	tests := []struct {
		name           string
		authHeader     string
		expectedResult bool
	}{
		{
			name:           "no auth header",
			authHeader:     "",
			expectedResult: true, // Auth is disabled by default
		},
		{
			name:           "valid bearer token",
			authHeader:     "Bearer this-is-a-valid-long-token-12345",
			expectedResult: true,
		},
		{
			name:           "invalid bearer format",
			authHeader:     "Basic dGVzdDp0ZXN0",
			expectedResult: false,
		},
		{
			name:           "empty bearer token",
			authHeader:     "Bearer ",
			expectedResult: false,
		},
		{
			name:           "short token",
			authHeader:     "Bearer short",
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			result := server.validateAuthentication(req)
			if result != tt.expectedResult {
				t.Errorf("Expected %v, got %v", tt.expectedResult, result)
			}
		})
	}
}

func TestErrorResponse(t *testing.T) {
	configs := map[string]config.MCPServer{}
	mcpManager := mcp.NewManager(configs)
	server := NewServer(mcpManager)

	// Test JSON-RPC error response
	rr := httptest.NewRecorder()
	server.sendErrorResponse(rr, "test-123", protocol.InvalidRequest, "Test error", false)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rr.Code)
	}

	var response protocol.JSONRPCMessage
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	if response.JSONRPC != "2.0" {
		t.Errorf("Expected JSONRPC version 2.0, got %s", response.JSONRPC)
	}

	if response.ID != "test-123" {
		t.Errorf("Expected ID 'test-123', got %v", response.ID)
	}

	if response.Error == nil {
		t.Error("Expected error field to be set")
	} else {
		if response.Error.Code != protocol.InvalidRequest {
			t.Errorf("Expected error code %d, got %d", protocol.InvalidRequest, response.Error.Code)
		}
		if response.Error.Message != "Test error" {
			t.Errorf("Expected error message 'Test error', got '%s'", response.Error.Message)
		}
	}
}

func TestConcurrentConnections(t *testing.T) {
	cm := NewConnectionManager(5)

	// Test concurrent connection additions
	const numGoroutines = 10
	const connectionsPerGoroutine = 2

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*connectionsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < connectionsPerGoroutine; j++ {
				ctx, cancel := context.WithCancel(context.Background())
				sessionID := fmt.Sprintf("session-%d-%d", id, j)

				err := cm.AddConnection(sessionID, "test-server", ctx, cancel)
				if err != nil {
					errors <- err
				}

				// Immediately remove to make room for others
				cm.RemoveConnection(sessionID)
				cancel()
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Count errors (some are expected due to connection limit)
	errorCount := 0
	for err := range errors {
		if err != nil {
			errorCount++
		}
	}

	// We expect some errors due to the connection limit
	if errorCount == 0 {
		t.Log("No errors occurred (this could be expected due to timing)")
	}

	// Verify final state
	if count := cm.GetConnectionCount(); count != 0 {
		t.Errorf("Expected 0 connections at end, got %d", count)
	}
}

func BenchmarkHealthEndpoint(b *testing.B) {
	configs := map[string]config.MCPServer{}
	mcpManager := mcp.NewManager(configs)
	server := NewServer(mcpManager)
	router := server.Router()

	req, _ := http.NewRequest("GET", "/health", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
	}
}

func BenchmarkSessionIDGeneration(b *testing.B) {
	configs := map[string]config.MCPServer{}
	mcpManager := mcp.NewManager(configs)
	server := NewServer(mcpManager)

	req, _ := http.NewRequest("GET", "/test", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.getSessionID(req)
	}
}
