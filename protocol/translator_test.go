package protocol

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestRemoteToMCP(t *testing.T) {
	translator := NewTranslator()

	tests := []struct {
		name           string
		input          RemoteMCPMessage
		expectedID     interface{}
		expectedMethod string
		shouldError    bool
	}{
		{
			name: "valid request message",
			input: RemoteMCPMessage{
				Type:   "request",
				ID:     "test-123",
				Method: "tools/list",
				Params: map[string]interface{}{"limit": 10},
			},
			expectedID:     "test-123",
			expectedMethod: "tools/list",
			shouldError:    false,
		},
		{
			name: "valid response message",
			input: RemoteMCPMessage{
				Type:   "response",
				ID:     42,
				Result: map[string]interface{}{"status": "ok"},
			},
			expectedID:  42,
			shouldError: false,
		},
		{
			name: "error response message",
			input: RemoteMCPMessage{
				Type: "response",
				ID:   "error-test",
				Error: &RPCError{
					Code:    -32600,
					Message: "Invalid Request",
				},
			},
			expectedID:  "error-test",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputBytes, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("Failed to marshal input: %v", err)
			}

			result, err := translator.RemoteToMCP(inputBytes)
			if tt.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			var jsonrpcMsg JSONRPCMessage
			if err := json.Unmarshal(result, &jsonrpcMsg); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}

			if jsonrpcMsg.JSONRPC != "2.0" {
				t.Errorf("Expected JSONRPC version 2.0, got %s", jsonrpcMsg.JSONRPC)
			}

			// Handle JSON number type conversion issues
			expectedIDStr := fmt.Sprintf("%v", tt.expectedID)
			actualIDStr := fmt.Sprintf("%v", jsonrpcMsg.ID)
			if actualIDStr != expectedIDStr {
				t.Errorf("Expected ID %v, got %v", tt.expectedID, jsonrpcMsg.ID)
			}

			if tt.expectedMethod != "" && jsonrpcMsg.Method != tt.expectedMethod {
				t.Errorf("Expected method %s, got %s", tt.expectedMethod, jsonrpcMsg.Method)
			}

			if tt.input.Error != nil && jsonrpcMsg.Error == nil {
				t.Error("Expected error in result but got none")
			}
		})
	}
}

func TestMCPToRemote(t *testing.T) {
	translator := NewTranslator()

	tests := []struct {
		name         string
		input        JSONRPCMessage
		expectedType string
		expectedID   interface{}
		shouldError  bool
	}{
		{
			name: "request message",
			input: JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      "test-456",
				Method:  "initialize",
				Params:  map[string]interface{}{"protocolVersion": "2024-11-05"},
			},
			expectedType: "request",
			expectedID:   "test-456",
			shouldError:  false,
		},
		{
			name: "response message with result",
			input: JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      789,
				Result:  map[string]interface{}{"capabilities": map[string]interface{}{}},
			},
			expectedType: "response",
			expectedID:   789,
			shouldError:  false,
		},
		{
			name: "response message with error",
			input: JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      "error-456",
				Error: &RPCError{
					Code:    -32601,
					Message: "Method not found",
				},
			},
			expectedType: "response",
			expectedID:   "error-456",
			shouldError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputBytes, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("Failed to marshal input: %v", err)
			}

			result, err := translator.MCPToRemote(inputBytes)
			if tt.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			var remoteMsg RemoteMCPMessage
			if err := json.Unmarshal(result, &remoteMsg); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}

			if remoteMsg.Type != tt.expectedType {
				t.Errorf("Expected type %s, got %s", tt.expectedType, remoteMsg.Type)
			}

			// Handle JSON number type conversion issues
			expectedIDStr := fmt.Sprintf("%v", tt.expectedID)
			actualIDStr := fmt.Sprintf("%v", remoteMsg.ID)
			if actualIDStr != expectedIDStr {
				t.Errorf("Expected ID %v, got %v", tt.expectedID, remoteMsg.ID)
			}
		})
	}
}

func TestHandleInitialize(t *testing.T) {
	translator := NewTranslator()
	sessionID := "test-session-123"

	tests := []struct {
		name            string
		params          InitializeParams
		expectedError   bool
		expectedVersion string
	}{
		{
			name: "valid initialize request",
			params: InitializeParams{
				ProtocolVersion: MCPProtocolVersion,
				Capabilities:    map[string]interface{}{"tools": map[string]interface{}{}},
				ClientInfo: ClientInfo{
					Name:    "test-client",
					Version: "1.0.0",
				},
			},
			expectedError:   false,
			expectedVersion: MCPProtocolVersion,
		},
		{
			name: "unsupported protocol version",
			params: InitializeParams{
				ProtocolVersion: "1999-01-01",
				Capabilities:    map[string]interface{}{},
				ClientInfo: ClientInfo{
					Name:    "old-client",
					Version: "0.1.0",
				},
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := translator.HandleInitialize(sessionID, tt.params)

			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.ProtocolVersion != tt.expectedVersion {
				t.Errorf("Expected protocol version %s, got %s", tt.expectedVersion, result.ProtocolVersion)
			}

			if result.ServerInfo.Name != ProxyServerName {
				t.Errorf("Expected server name %s, got %s", ProxyServerName, result.ServerInfo.Name)
			}

			// Check that connection state was created
			if !translator.IsInitialized(sessionID) {
				// The connection should not be initialized until HandleInitialized is called
				state, exists := translator.GetConnectionState(sessionID)
				if !exists {
					t.Error("Expected connection state to be created")
				}
				if state.Initialized {
					t.Error("Connection should not be initialized yet")
				}
			}
		})
	}
}

func TestHandleInitialized(t *testing.T) {
	translator := NewTranslator()
	sessionID := "test-session-456"

	// First initialize the session
	params := InitializeParams{
		ProtocolVersion: MCPProtocolVersion,
		Capabilities:    map[string]interface{}{},
		ClientInfo: ClientInfo{
			Name:    "test-client",
			Version: "1.0.0",
		},
	}

	_, err := translator.HandleInitialize(sessionID, params)
	if err != nil {
		t.Fatalf("Failed to initialize session: %v", err)
	}

	// Test HandleInitialized
	err = translator.HandleInitialized(sessionID)
	if err != nil {
		t.Fatalf("Unexpected error in HandleInitialized: %v", err)
	}

	// Check that connection is now initialized
	if !translator.IsInitialized(sessionID) {
		t.Error("Expected connection to be initialized")
	}

	// Test with non-existent session
	err = translator.HandleInitialized("non-existent-session")
	if err == nil {
		t.Error("Expected error for non-existent session")
	}
}

func TestIsHandshakeMessage(t *testing.T) {
	translator := NewTranslator()

	tests := []struct {
		method   string
		expected bool
	}{
		{"initialize", true},
		{"notifications/initialized", true},
		{"tools/list", false},
		{"resources/read", false},
		{"prompts/get", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			result := translator.IsHandshakeMessage(tt.method)
			if result != tt.expected {
				t.Errorf("Expected %v for method %s, got %v", tt.expected, tt.method, result)
			}
		})
	}
}

func TestCreateErrorResponse(t *testing.T) {
	translator := NewTranslator()

	tests := []struct {
		name        string
		id          interface{}
		code        int
		message     string
		isRemoteMCP bool
	}{
		{
			name:        "JSON-RPC error response",
			id:          "test-123",
			code:        InvalidRequest,
			message:     "Test error message",
			isRemoteMCP: false,
		},
		{
			name:        "Remote MCP error response",
			id:          456,
			code:        MethodNotFound,
			message:     "Method not found",
			isRemoteMCP: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := translator.CreateErrorResponse(tt.id, tt.code, tt.message, tt.isRemoteMCP)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tt.isRemoteMCP {
				var remoteMsg RemoteMCPMessage
				if err := json.Unmarshal(result, &remoteMsg); err != nil {
					t.Fatalf("Failed to unmarshal remote MCP message: %v", err)
				}

				if remoteMsg.Type != "response" {
					t.Errorf("Expected type 'response', got %s", remoteMsg.Type)
				}

				expectedIDStr := fmt.Sprintf("%v", tt.id)
				actualIDStr := fmt.Sprintf("%v", remoteMsg.ID)
				if actualIDStr != expectedIDStr {
					t.Errorf("Expected ID %v, got %v", tt.id, remoteMsg.ID)
				}

				if remoteMsg.Error == nil {
					t.Error("Expected error field to be set")
				} else {
					if remoteMsg.Error.Code != tt.code {
						t.Errorf("Expected error code %d, got %d", tt.code, remoteMsg.Error.Code)
					}
					if remoteMsg.Error.Message != tt.message {
						t.Errorf("Expected error message %s, got %s", tt.message, remoteMsg.Error.Message)
					}
				}
			} else {
				var jsonrpcMsg JSONRPCMessage
				if err := json.Unmarshal(result, &jsonrpcMsg); err != nil {
					t.Fatalf("Failed to unmarshal JSON-RPC message: %v", err)
				}

				if jsonrpcMsg.JSONRPC != "2.0" {
					t.Errorf("Expected JSONRPC version 2.0, got %s", jsonrpcMsg.JSONRPC)
				}

				expectedIDStr := fmt.Sprintf("%v", tt.id)
				actualIDStr := fmt.Sprintf("%v", jsonrpcMsg.ID)
				if actualIDStr != expectedIDStr {
					t.Errorf("Expected ID %v, got %v", tt.id, jsonrpcMsg.ID)
				}

				if jsonrpcMsg.Error == nil {
					t.Error("Expected error field to be set")
				}
			}
		})
	}
}

func TestConnectionStateManagement(t *testing.T) {
	translator := NewTranslator()
	sessionID1 := "session-1"
	sessionID2 := "session-2"

	// Initialize first session
	params := InitializeParams{
		ProtocolVersion: MCPProtocolVersion,
		Capabilities:    map[string]interface{}{},
		ClientInfo: ClientInfo{
			Name:    "client-1",
			Version: "1.0.0",
		},
	}

	_, err := translator.HandleInitialize(sessionID1, params)
	if err != nil {
		t.Fatalf("Failed to initialize session 1: %v", err)
	}

	// Check connection state exists
	state1, exists := translator.GetConnectionState(sessionID1)
	if !exists {
		t.Error("Expected session 1 state to exist")
	}

	if state1.SessionID != sessionID1 {
		t.Errorf("Expected session ID %s, got %s", sessionID1, state1.SessionID)
	}

	// Initialize second session
	_, err = translator.HandleInitialize(sessionID2, params)
	if err != nil {
		t.Fatalf("Failed to initialize session 2: %v", err)
	}

	// Both sessions should exist
	_, exists1 := translator.GetConnectionState(sessionID1)
	_, exists2 := translator.GetConnectionState(sessionID2)

	if !exists1 || !exists2 {
		t.Error("Expected both session states to exist")
	}

	// Remove first session
	translator.RemoveConnection(sessionID1)

	_, exists1After := translator.GetConnectionState(sessionID1)
	_, exists2After := translator.GetConnectionState(sessionID2)

	if exists1After {
		t.Error("Expected session 1 to be removed")
	}

	if !exists2After {
		t.Error("Expected session 2 to still exist")
	}
}

func TestValidateMessage(t *testing.T) {
	translator := NewTranslator()

	tests := []struct {
		name        string
		input       string
		isRemoteMCP bool
		shouldError bool
	}{
		{
			name:        "valid JSON-RPC message",
			input:       `{"jsonrpc":"2.0","id":"test","method":"initialize"}`,
			isRemoteMCP: false,
			shouldError: false,
		},
		{
			name:        "invalid JSON-RPC version",
			input:       `{"jsonrpc":"1.0","id":"test","method":"initialize"}`,
			isRemoteMCP: false,
			shouldError: true,
		},
		{
			name:        "valid Remote MCP message",
			input:       `{"type":"request","id":"test","method":"initialize"}`,
			isRemoteMCP: true,
			shouldError: false,
		},
		{
			name:        "invalid JSON",
			input:       `{"invalid json`,
			isRemoteMCP: false,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := translator.ValidateMessage([]byte(tt.input), tt.isRemoteMCP)

			if tt.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}
