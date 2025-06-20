package protocol

import (
	"encoding/json"
	"fmt"
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
	Type    string      `json:"type"`
	ID      interface{} `json:"id,omitempty"`
	Method  string      `json:"method,omitempty"`
	Params  interface{} `json:"params,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// Translator handles protocol translation between Remote MCP and local MCP
type Translator struct {
	// Could add state tracking here if needed
}

// NewTranslator creates a new protocol translator
func NewTranslator() *Translator {
	return &Translator{}
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