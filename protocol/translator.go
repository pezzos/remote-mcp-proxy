package protocol

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
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

	// Transform tool names back for tool calls (snake_case to original format)
	params := remoteMsg.Params
	if remoteMsg.Method == "tools/call" && params != nil {
		params = t.denormalizeToolNames(params)
	}

	// Convert to JSON-RPC format
	jsonrpcMsg := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      remoteMsg.ID,
		Method:  remoteMsg.Method,
		Params:  params,
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

	// Enhanced logging for tool discovery debugging
	if messageType == "response" && jsonrpcMsg.ID != nil {
		log.Printf("=== TOOL DISCOVERY DEBUG ===")
		log.Printf("DEBUG: Processing MCP response - ID: %v, Method: %s", jsonrpcMsg.ID, jsonrpcMsg.Method)
		log.Printf("DEBUG: Raw MCP response: %s", string(mcpData))
		log.Printf("DEBUG: Has result: %v, Has error: %v", jsonrpcMsg.Result != nil, jsonrpcMsg.Error != nil)
	}

	// Transform tool names in tools/list responses for Claude.ai compatibility
	result := jsonrpcMsg.Result
	if messageType == "response" && result != nil {
		// Check if this is a tools/list response for enhanced logging
		if resultMap, ok := result.(map[string]interface{}); ok {
			if tools, exists := resultMap["tools"]; exists {
				log.Printf("DEBUG: Found tools/list response with %d tools before normalization", func() int {
					if toolsList, ok := tools.([]interface{}); ok {
						return len(toolsList)
					}
					return 0
				}())
				log.Printf("DEBUG: Tools before normalization: %+v", tools)
			}
		}

		result = t.normalizeToolNames(result)

		// Log after normalization
		if resultMap, ok := result.(map[string]interface{}); ok {
			if tools, exists := resultMap["tools"]; exists {
				log.Printf("DEBUG: Tools after normalization: %+v", tools)
			}
		}
	}

	// Convert to Remote MCP format
	remoteMsg := RemoteMCPMessage{
		Type:   messageType,
		ID:     jsonrpcMsg.ID,
		Method: jsonrpcMsg.Method,
		Params: jsonrpcMsg.Params,
		Result: result,
		Error:  jsonrpcMsg.Error,
	}

	remoteMsgBytes, err := json.Marshal(remoteMsg)
	if err != nil {
		return nil, err
	}

	// Enhanced logging for Remote MCP message format validation
	if messageType == "response" && jsonrpcMsg.ID != nil {
		log.Printf("DEBUG: Final Remote MCP message: %s", string(remoteMsgBytes))
		log.Printf("DEBUG: Remote MCP message type: %s, ID: %v", remoteMsg.Type, remoteMsg.ID)
		log.Printf("=== TOOL DISCOVERY DEBUG END ===")
	}

	return remoteMsgBytes, nil
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

// normalizeToolNames transforms tool names to be Claude.ai compatible (snake_case)
func (t *Translator) normalizeToolNames(result interface{}) interface{} {
	// Handle tools/list response format
	if resultMap, ok := result.(map[string]interface{}); ok {
		if tools, exists := resultMap["tools"]; exists {
			if toolsList, ok := tools.([]interface{}); ok {
				normalizedTools := make([]interface{}, len(toolsList))
				for i, tool := range toolsList {
					if toolMap, ok := tool.(map[string]interface{}); ok {
						// Create a copy of the tool map
						normalizedTool := make(map[string]interface{})
						for k, v := range toolMap {
							normalizedTool[k] = v
						}

						// Transform the tool name: convert hyphens to underscores and lowercase
						if name, exists := normalizedTool["name"]; exists {
							if nameStr, ok := name.(string); ok {
								normalizedName := strings.ToLower(strings.ReplaceAll(nameStr, "-", "_"))
								normalizedTool["name"] = normalizedName
							}
						}
						normalizedTools[i] = normalizedTool
					} else {
						normalizedTools[i] = tool
					}
				}

				// Update the result map with normalized tools
				normalizedResult := make(map[string]interface{})
				for k, v := range resultMap {
					normalizedResult[k] = v
				}
				normalizedResult["tools"] = normalizedTools
				return normalizedResult
			}
		}
	}

	// Return original result if no tools found or transformation not applicable
	return result
}

// denormalizeToolNames transforms tool names back from snake_case to original format for tool calls
func (t *Translator) denormalizeToolNames(params interface{}) interface{} {
	// Handle tools/call request format
	if paramsMap, ok := params.(map[string]interface{}); ok {
		if name, exists := paramsMap["name"]; exists {
			if nameStr, ok := name.(string); ok {
				// Convert snake_case back to original API format
				// api_get_user -> API-get-user
				originalName := strings.ReplaceAll(nameStr, "_", "-")
				if strings.HasPrefix(originalName, "api-") {
					originalName = "API" + originalName[3:]
				}

				// Create a copy of the params map with the transformed name
				normalizedParams := make(map[string]interface{})
				for k, v := range paramsMap {
					normalizedParams[k] = v
				}
				normalizedParams["name"] = originalName
				return normalizedParams
			}
		}
	}

	// Return original params if no transformation needed
	return params
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
