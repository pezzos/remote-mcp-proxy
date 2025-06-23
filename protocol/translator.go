package protocol

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
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

// PendingRequest tracks a request waiting for a response
type PendingRequest struct {
	Method    string
	Timestamp time.Time
}

// ConnectionState tracks the state of an MCP connection
type ConnectionState struct {
	Initialized     bool
	ProtocolVersion string
	Capabilities    map[string]interface{}
	SessionID       string
	PendingRequests map[interface{}]*PendingRequest // Maps request ID to request info
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
		PendingRequests: make(map[interface{}]*PendingRequest),
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

// ShouldProvideFallback checks if we should provide a fallback response for unsupported methods
func (t *Translator) ShouldProvideFallback(method string) bool {
	fallbackMethods := []string{
		"resources/list",
		"resources/read",
		"prompts/list",
		"prompts/get",
	}

	for _, fm := range fallbackMethods {
		if method == fm {
			return true
		}
	}
	return false
}

// CreateFallbackResponse creates a fallback response for unsupported methods
func (t *Translator) CreateFallbackResponse(id interface{}, method string) ([]byte, error) {
	var result interface{}

	switch method {
	case "resources/list":
		result = map[string]interface{}{
			"resources": []interface{}{},
		}
	case "resources/read":
		return t.CreateErrorResponse(id, MethodNotFound, "Resource not found", false)
	case "prompts/list":
		result = map[string]interface{}{
			"prompts": []interface{}{},
		}
	case "prompts/get":
		return t.CreateErrorResponse(id, MethodNotFound, "Prompt not found", false)
	default:
		return t.CreateErrorResponse(id, MethodNotFound, "Method not found", false)
	}

	jsonrpcMsg := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}

	return json.Marshal(jsonrpcMsg)
}

// TrackRequest tracks a pending request for fallback handling
func (t *Translator) TrackRequest(sessionID string, requestID interface{}, method string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if state, exists := t.connections[sessionID]; exists {
		state.PendingRequests[requestID] = &PendingRequest{
			Method:    method,
			Timestamp: time.Now(),
		}
	}
}

// GetAndClearPendingMethod gets and removes a pending request method
func (t *Translator) GetAndClearPendingMethod(sessionID string, requestID interface{}) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if state, exists := t.connections[sessionID]; exists {
		if request, found := state.PendingRequests[requestID]; found {
			delete(state.PendingRequests, requestID)
			return request.Method, true
		}
	}
	return "", false
}

// HandleMethodNotFoundError processes "Method not found" errors and provides fallbacks when appropriate
func (t *Translator) HandleMethodNotFoundError(sessionID string, response []byte) ([]byte, bool) {
	var mcpResponse JSONRPCMessage
	if err := json.Unmarshal(response, &mcpResponse); err != nil {
		return response, false
	}

	// Check if this is a "Method not found" error
	if mcpResponse.Error == nil || mcpResponse.Error.Code != MethodNotFound {
		return response, false
	}

	// Get the original method for this request ID
	method, found := t.GetAndClearPendingMethod(sessionID, mcpResponse.ID)
	if !found || !t.ShouldProvideFallback(method) {
		return response, false
	}

	// Create fallback response
	fallbackResponse, err := t.CreateFallbackResponse(mcpResponse.ID, method)
	if err != nil {
		return response, false
	}

	return fallbackResponse, true
}

// CheckTimeouts checks for timed-out requests and generates fallback responses
func (t *Translator) CheckTimeouts(sessionID string, timeoutDuration time.Duration) [][]byte {
	t.mu.Lock()
	defer t.mu.Unlock()

	var fallbackMessages [][]byte

	state, exists := t.connections[sessionID]
	if !exists {
		return fallbackMessages
	}

	now := time.Now()
	var expiredRequests []interface{}

	// Find expired requests
	for requestID, request := range state.PendingRequests {
		if now.Sub(request.Timestamp) > timeoutDuration && t.ShouldProvideFallback(request.Method) {
			expiredRequests = append(expiredRequests, requestID)
		}
	}

	// Generate fallback responses for expired requests
	for _, requestID := range expiredRequests {
		request := state.PendingRequests[requestID]
		if fallbackResponse, err := t.CreateFallbackResponse(requestID, request.Method); err == nil {
			fallbackMessages = append(fallbackMessages, fallbackResponse)
		}
		delete(state.PendingRequests, requestID)
	}

	return fallbackMessages
}
