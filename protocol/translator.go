package protocol

import (
	"encoding/json"
	"fmt"
	"sync"
)

// JSONRPCMessage represents a JSON-RPC 2.0 message
type JSONRPCMessage struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Method  string      `json:"method,omitempty"`
	Params  interface{} `json:"params,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// RemoteMCPMessage represents a Remote MCP protocol message
type RemoteMCPMessage struct {
	Type   string      `json:"type"`
	ID     interface{} `json:"id,omitempty"`
	Method string      `json:"method,omitempty"`
	Params interface{} `json:"params,omitempty"`
	Result interface{} `json:"result,omitempty"`
	Error  *RPCError   `json:"error,omitempty"`
}

// InitializeParams represents parameters for the initialize request
type InitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ClientInfo      ClientInfo             `json:"clientInfo"`
}

// InitializeResult represents the result of the initialize request
type InitializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ServerInfo      ServerInfo             `json:"serverInfo"`
}

// ClientInfo represents information about the client
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerInfo represents information about the server
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ConnectionState tracks the state of an MCP connection
type ConnectionState struct {
	Initialized     bool
	ProtocolVersion string
	Capabilities    map[string]interface{}
	SessionID       string
}

// Translator handles protocol translation between Remote MCP and local MCP
type Translator struct {
	connections map[string]*ConnectionState
	mu          sync.RWMutex
}

// NewTranslator creates a new protocol translator
func NewTranslator() *Translator {
	return &Translator{
		connections: make(map[string]*ConnectionState),
	}
}

// RemoteToMCP converts a Remote MCP message to local MCP JSON-RPC format
func (t *Translator) RemoteToMCP(remoteMCPData []byte) ([]byte, error) {
	var remoteMsg RemoteMCPMessage
	if err := json.Unmarshal(remoteMCPData, &remoteMsg); err != nil {
		return nil, fmt.Errorf("failed to parse Remote MCP message: %w", err)
	}

	// Convert to JSON-RPC format
	jsonrpcMsg := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      remoteMsg.ID,
		Method:  remoteMsg.Method,
		Params:  remoteMsg.Params,
		Result:  remoteMsg.Result,
		Error:   remoteMsg.Error,
	}

	return json.Marshal(jsonrpcMsg)
}

// MCPToRemote converts a local MCP JSON-RPC message to Remote MCP format
func (t *Translator) MCPToRemote(mcpData []byte) ([]byte, error) {
	var jsonrpcMsg JSONRPCMessage
	if err := json.Unmarshal(mcpData, &jsonrpcMsg); err != nil {
		return nil, fmt.Errorf("failed to parse MCP JSON-RPC message: %w", err)
	}

	// Determine message type
	messageType := "request"
	if jsonrpcMsg.Result != nil || jsonrpcMsg.Error != nil {
		messageType = "response"
	}

	// Convert to Remote MCP format
	remoteMsg := RemoteMCPMessage{
		Type:   messageType,
		ID:     jsonrpcMsg.ID,
		Method: jsonrpcMsg.Method,
		Params: jsonrpcMsg.Params,
		Result: jsonrpcMsg.Result,
		Error:  jsonrpcMsg.Error,
	}

	return json.Marshal(remoteMsg)
}

// ValidateMessage validates that a message conforms to expected format
func (t *Translator) ValidateMessage(data []byte, isRemoteMCP bool) error {
	if isRemoteMCP {
		var msg RemoteMCPMessage
		return json.Unmarshal(data, &msg)
	} else {
		var msg JSONRPCMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return err
		}

		// Validate JSON-RPC 2.0 format
		if msg.JSONRPC != "2.0" {
			return fmt.Errorf("invalid JSON-RPC version: %s", msg.JSONRPC)
		}

		return nil
	}
}

// CreateErrorResponse creates an error response message
func (t *Translator) CreateErrorResponse(id interface{}, code int, message string, isRemoteMCP bool) ([]byte, error) {
	rpcError := &RPCError{
		Code:    code,
		Message: message,
	}

	if isRemoteMCP {
		remoteMsg := RemoteMCPMessage{
			Type:  "response",
			ID:    id,
			Error: rpcError,
		}
		return json.Marshal(remoteMsg)
	} else {
		jsonrpcMsg := JSONRPCMessage{
			JSONRPC: "2.0",
			ID:      id,
			Error:   rpcError,
		}
		return json.Marshal(jsonrpcMsg)
	}
}

// Standard JSON-RPC error codes
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// MCP Protocol constants
const (
	MCPProtocolVersion = "2024-11-05"
	ProxyServerName    = "remote-mcp-proxy"
	ProxyServerVersion = "1.0.0"
)

// HandleInitialize processes the MCP initialize request
func (t *Translator) HandleInitialize(sessionID string, params InitializeParams) (*InitializeResult, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Validate protocol version
	if params.ProtocolVersion != MCPProtocolVersion {
		return nil, fmt.Errorf("unsupported protocol version: %s", params.ProtocolVersion)
	}

	// Create or update connection state
	state := &ConnectionState{
		Initialized:     false,
		ProtocolVersion: MCPProtocolVersion,
		Capabilities:    make(map[string]interface{}),
		SessionID:       sessionID,
	}

	// Set basic capabilities for proxy
	state.Capabilities = map[string]interface{}{
		"tools":     map[string]interface{}{},
		"resources": map[string]interface{}{},
		"prompts":   map[string]interface{}{},
	}

	t.connections[sessionID] = state

	return &InitializeResult{
		ProtocolVersion: MCPProtocolVersion,
		Capabilities:    state.Capabilities,
		ServerInfo: ServerInfo{
			Name:    ProxyServerName,
			Version: ProxyServerVersion,
		},
	}, nil
}

// HandleInitialized processes the MCP initialized notification
func (t *Translator) HandleInitialized(sessionID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	state, exists := t.connections[sessionID]
	if !exists {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	state.Initialized = true
	return nil
}

// IsInitialized checks if a session has completed initialization
func (t *Translator) IsInitialized(sessionID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	state, exists := t.connections[sessionID]
	return exists && state.Initialized
}

// GetConnectionState returns the connection state for a session
func (t *Translator) GetConnectionState(sessionID string) (*ConnectionState, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	state, exists := t.connections[sessionID]
	return state, exists
}

// RemoveConnection removes a connection state
func (t *Translator) RemoveConnection(sessionID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.connections, sessionID)
}

// IsHandshakeMessage checks if a message is part of the handshake process
func (t *Translator) IsHandshakeMessage(method string) bool {
	return method == "initialize" || method == "notifications/initialized"
}
