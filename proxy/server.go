package proxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"remote-mcp-proxy/mcp"
	"remote-mcp-proxy/protocol"
)

// Server represents the HTTP proxy server
type Server struct {
	mcpManager        *mcp.Manager
	translator        *protocol.Translator
	connectionManager *ConnectionManager
}

// ConnectionManager manages active SSE connections
type ConnectionManager struct {
	connections    map[string]*ConnectionInfo
	maxConnections int
	mu             sync.RWMutex
}

// ConnectionInfo holds information about an active connection
type ConnectionInfo struct {
	SessionID   string
	ServerName  string
	ConnectedAt time.Time
	Context     context.Context
	Cancel      context.CancelFunc
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(maxConnections int) *ConnectionManager {
	return &ConnectionManager{
		connections:    make(map[string]*ConnectionInfo),
		maxConnections: maxConnections,
	}
}

// AddConnection adds a new connection to the manager
func (cm *ConnectionManager) AddConnection(sessionID, serverName string, ctx context.Context, cancel context.CancelFunc) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Check connection limit
	if len(cm.connections) >= cm.maxConnections {
		log.Printf("WARNING: Connection limit reached (%d), rejecting new connection for session %s", cm.maxConnections, sessionID)
		return fmt.Errorf("connection limit reached: %d", cm.maxConnections)
	}

	// Add connection
	cm.connections[sessionID] = &ConnectionInfo{
		SessionID:   sessionID,
		ServerName:  serverName,
		ConnectedAt: time.Now(),
		Context:     ctx,
		Cancel:      cancel,
	}

	log.Printf("INFO: Added connection for session %s (total: %d/%d)", sessionID, len(cm.connections), cm.maxConnections)
	return nil
}

// RemoveConnection removes a connection from the manager
func (cm *ConnectionManager) RemoveConnection(sessionID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if conn, exists := cm.connections[sessionID]; exists {
		// Cancel the connection context
		if conn.Cancel != nil {
			conn.Cancel()
		}
		delete(cm.connections, sessionID)
		log.Printf("INFO: Removed connection for session %s (remaining: %d)", sessionID, len(cm.connections))
	}
}

// GetConnectionCount returns the current number of active connections
func (cm *ConnectionManager) GetConnectionCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.connections)
}

// GetConnections returns a copy of all active connections
func (cm *ConnectionManager) GetConnections() map[string]ConnectionInfo {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	result := make(map[string]ConnectionInfo)
	for k, v := range cm.connections {
		result[k] = *v
	}
	return result
}

// CleanupStaleConnections removes connections that have been inactive for too long
func (cm *ConnectionManager) CleanupStaleConnections(maxAge time.Duration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	var removed []string

	for sessionID, conn := range cm.connections {
		if now.Sub(conn.ConnectedAt) > maxAge {
			if conn.Cancel != nil {
				conn.Cancel()
			}
			delete(cm.connections, sessionID)
			removed = append(removed, sessionID)
		}
	}

	if len(removed) > 0 {
		log.Printf("INFO: Cleaned up %d stale connections: %v", len(removed), removed)
	}
}

// NewServer creates a new proxy server
func NewServer(mcpManager *mcp.Manager) *Server {
	const maxConnections = 100 // Configurable connection limit

	server := &Server{
		mcpManager:        mcpManager,
		translator:        protocol.NewTranslator(),
		connectionManager: NewConnectionManager(maxConnections),
	}

	// Start background cleanup routine
	go server.startConnectionCleanup()

	log.Printf("INFO: Created proxy server with max %d connections", maxConnections)
	return server
}

// startConnectionCleanup starts a background goroutine to clean up stale connections
func (s *Server) startConnectionCleanup() {
	ticker := time.NewTicker(30 * time.Second) // Cleanup every 30 seconds
	defer ticker.Stop()

	maxAge := 10 * time.Minute // Remove connections older than 10 minutes

	for {
		select {
		case <-ticker.C:
			s.connectionManager.CleanupStaleConnections(maxAge)
		}
	}
}

// Router returns the HTTP router with all routes configured
func (s *Server) Router() http.Handler {
	r := mux.NewRouter()

	// Health check endpoint
	r.HandleFunc("/health", s.handleHealth).Methods("GET", "OPTIONS")

	// MCP server endpoints - pattern: /{server-name}/sse
	r.HandleFunc("/{server:[^/]+}/sse", s.handleMCPRequest).Methods("GET", "POST")

	// Add CORS middleware
	r.Use(s.corsMiddleware)

	return r
}

// handleHealth returns server health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write([]byte(`{"status":"healthy"}`)); err != nil {
		log.Printf("ERROR: Failed to write health response: %v", err)
	} else {
		log.Printf("DEBUG: Health check response sent successfully")
	}
}

// handleMCPRequest handles Remote MCP requests and forwards them to local MCP servers
func (s *Server) handleMCPRequest(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverName := vars["server"]

	log.Printf("Handling MCP request for server: %s", serverName)

	// Validate authentication
	if !s.validateAuthentication(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get the MCP server
	mcpServer, exists := s.mcpManager.GetServer(serverName)
	if !exists {
		http.Error(w, fmt.Sprintf("MCP server '%s' not found", serverName), http.StatusNotFound)
		return
	}

	// Handle based on request method
	switch r.Method {
	case "GET":
		s.handleSSEConnection(w, r, mcpServer)
	case "POST":
		s.handleMCPMessage(w, r, mcpServer)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSSEConnection establishes a Server-Sent Events connection
func (s *Server) handleSSEConnection(w http.ResponseWriter, r *http.Request, mcpServer *mcp.Server) {
	// Get or generate session ID
	sessionID := s.getSessionID(r)

	// Create cancellable context for this connection
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Check connection limits and add to manager
	if err := s.connectionManager.AddConnection(sessionID, mcpServer.Name, ctx, cancel); err != nil {
		log.Printf("ERROR: Failed to add connection for session %s: %v", sessionID, err)
		http.Error(w, "Too many connections", http.StatusTooManyRequests)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Session-ID", sessionID)

	// Send initial connection event with connection count
	if _, err := fmt.Fprintf(w, "event: connected\n"); err != nil {
		log.Printf("ERROR: Failed to write SSE connected event: %v", err)
		s.connectionManager.RemoveConnection(sessionID)
		return
	}

	connectionData := fmt.Sprintf(`{"type":"connected","server":"%s","sessionId":"%s","connectionCount":%d}`,
		mcpServer.Name, sessionID, s.connectionManager.GetConnectionCount())

	if _, err := fmt.Fprintf(w, "data: %s\n\n", connectionData); err != nil {
		log.Printf("ERROR: Failed to write SSE connected data: %v", err)
		s.connectionManager.RemoveConnection(sessionID)
		return
	}

	// Flush to send the event immediately
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Clean up when connection closes
	defer func() {
		s.connectionManager.RemoveConnection(sessionID)
		s.translator.RemoveConnection(sessionID)
		log.Printf("INFO: SSE connection cleanup completed for server %s, session %s", mcpServer.Name, sessionID)
	}()

	// Create a ticker for periodic checks and timeouts
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	log.Printf("INFO: Starting SSE message loop for server %s, session %s", mcpServer.Name, sessionID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("INFO: SSE context cancelled for server %s, session %s", mcpServer.Name, sessionID)
			return
		case <-ticker.C:
			// Only process messages if connection is initialized
			if !s.translator.IsInitialized(sessionID) {
				continue
			}

			// Create a timeout context for reading messages
			readCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)

			// Read message from MCP server with timeout
			message, err := mcpServer.ReadMessage(readCtx)
			cancel()

			if err != nil {
				// Handle different types of errors appropriately
				if err == context.DeadlineExceeded {
					// Timeout is normal, just continue
					continue
				} else if err == context.Canceled {
					log.Printf("INFO: Read cancelled for server %s", mcpServer.Name)
					return
				} else if err.Error() != "EOF" {
					log.Printf("ERROR: Error reading from MCP server %s: %v", mcpServer.Name, err)
				}
				continue
			}

			// Translate and send message
			remoteMCPMessage, err := s.translator.MCPToRemote(message)
			if err != nil {
				log.Printf("ERROR: Error translating MCP message for server %s: %v", mcpServer.Name, err)
				continue
			}

			// Write SSE event with error handling
			if _, err := fmt.Fprintf(w, "event: message\n"); err != nil {
				log.Printf("ERROR: Failed to write SSE event header for server %s: %v", mcpServer.Name, err)
				return
			}

			if _, err := fmt.Fprintf(w, "data: %s\n\n", string(remoteMCPMessage)); err != nil {
				log.Printf("ERROR: Failed to write SSE data for server %s: %v", mcpServer.Name, err)
				return
			}

			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			} else {
				log.Printf("WARNING: ResponseWriter does not support flushing for server %s", mcpServer.Name)
			}

			log.Printf("DEBUG: Sent SSE message for server %s", mcpServer.Name)
		}
	}
}

// handleMCPMessage handles POST requests with MCP messages
func (s *Server) handleMCPMessage(w http.ResponseWriter, r *http.Request, mcpServer *mcp.Server) {
	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	// Parse the JSON-RPC message to check if it's a handshake message
	var jsonrpcMsg protocol.JSONRPCMessage
	if err := json.Unmarshal(body, &jsonrpcMsg); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON-RPC message: %v", err), http.StatusBadRequest)
		return
	}

	// Generate or get session ID
	sessionID := s.getSessionID(r)

	// Handle handshake messages
	if s.translator.IsHandshakeMessage(jsonrpcMsg.Method) {
		s.handleHandshakeMessage(w, r, sessionID, &jsonrpcMsg)
		return
	}

	// Check if connection is initialized
	if !s.translator.IsInitialized(sessionID) {
		s.sendErrorResponse(w, jsonrpcMsg.ID, protocol.InvalidRequest, "Connection not initialized", false)
		return
	}

	// Translate Remote MCP message to local MCP format
	mcpMessage, err := s.translator.RemoteToMCP(body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to translate message: %v", err), http.StatusBadRequest)
		return
	}

	// Send message to MCP server
	if err := mcpServer.SendMessage(mcpMessage); err != nil {
		http.Error(w, fmt.Sprintf("Failed to send message to MCP server: %v", err), http.StatusInternalServerError)
		return
	}

	// Return success response (HTTP 202 Accepted for MCP)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)

	if _, err := w.Write([]byte(`{"status":"accepted"}`)); err != nil {
		log.Printf("ERROR: Failed to write MCP message response: %v", err)
	} else {
		log.Printf("INFO: MCP message accepted and forwarded successfully")
	}
}

// corsMiddleware adds CORS headers to all responses
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate origin
		if !s.validateOrigin(r) {
			http.Error(w, "Origin not allowed", http.StatusForbidden)
			return
		}

		// Set CORS headers
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-ID")
		w.Header().Set("Access-Control-Expose-Headers", "X-Session-ID")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// logRequest logs HTTP requests for debugging
func (s *Server) logRequest(r *http.Request) {
	log.Printf("%s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

	// Log headers for debugging Remote MCP protocol
	for name, values := range r.Header {
		if strings.HasPrefix(strings.ToLower(name), "x-") ||
			strings.Contains(strings.ToLower(name), "mcp") {
			log.Printf("Header %s: %v", name, values)
		}
	}
}

// getSessionID generates or retrieves a session ID for the request
func (s *Server) getSessionID(r *http.Request) string {
	// Try to get session ID from header
	if sessionID := r.Header.Get("X-Session-ID"); sessionID != "" {
		log.Printf("DEBUG: Using existing session ID: %s", sessionID)
		return sessionID
	}

	// Generate a new session ID
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		log.Printf("ERROR: Failed to generate random session ID: %v", err)
		// Fallback to a simple timestamp-based ID
		return fmt.Sprintf("session-%d", time.Now().UnixNano())
	}

	sessionID := hex.EncodeToString(bytes)
	log.Printf("INFO: Generated new session ID: %s", sessionID)
	return sessionID
}

// handleHandshakeMessage handles MCP handshake messages (initialize and initialized)
func (s *Server) handleHandshakeMessage(w http.ResponseWriter, r *http.Request, sessionID string, msg *protocol.JSONRPCMessage) {
	switch msg.Method {
	case "initialize":
		s.handleInitialize(w, sessionID, msg)
	case "notifications/initialized":
		s.handleInitialized(w, sessionID, msg)
	default:
		s.sendErrorResponse(w, msg.ID, protocol.MethodNotFound, "Unknown handshake method", false)
	}
}

// handleInitialize handles the initialize request
func (s *Server) handleInitialize(w http.ResponseWriter, sessionID string, msg *protocol.JSONRPCMessage) {
	// Parse initialize parameters
	var params protocol.InitializeParams
	if msg.Params != nil {
		paramsBytes, _ := json.Marshal(msg.Params)
		if err := json.Unmarshal(paramsBytes, &params); err != nil {
			s.sendErrorResponse(w, msg.ID, protocol.InvalidParams, "Invalid initialize parameters", false)
			return
		}
	}

	// Handle initialize request
	result, err := s.translator.HandleInitialize(sessionID, params)
	if err != nil {
		s.sendErrorResponse(w, msg.ID, protocol.InvalidRequest, err.Error(), false)
		return
	}

	// Send initialize response
	response := protocol.JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      msg.ID,
		Result:  result,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Session-ID", sessionID)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	log.Printf("Sent initialize response for session %s", sessionID)
}

// handleInitialized handles the initialized notification
func (s *Server) handleInitialized(w http.ResponseWriter, sessionID string, msg *protocol.JSONRPCMessage) {
	// Handle initialized notification
	if err := s.translator.HandleInitialized(sessionID); err != nil {
		s.sendErrorResponse(w, msg.ID, protocol.InvalidRequest, err.Error(), false)
		return
	}

	// Send HTTP 202 Accepted for notification
	w.WriteHeader(http.StatusAccepted)

	log.Printf("Connection initialized for session %s", sessionID)
}

// sendErrorResponse sends a JSON-RPC error response
func (s *Server) sendErrorResponse(w http.ResponseWriter, id interface{}, code int, message string, isRemoteMCP bool) {
	log.Printf("ERROR: Sending error response - Code: %d, Message: %s", code, message)

	errorResponse, err := s.translator.CreateErrorResponse(id, code, message, isRemoteMCP)
	if err != nil {
		log.Printf("ERROR: Failed to create error response: %v", err)
		http.Error(w, "Failed to create error response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(errorResponse); err != nil {
		log.Printf("ERROR: Failed to write error response: %v", err)
	} else {
		log.Printf("DEBUG: Error response sent successfully")
	}
}

// validateAuthentication validates the authentication for the request
func (s *Server) validateAuthentication(r *http.Request) bool {
	// Check for Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		// For now, allow requests without authentication (can be configured later)
		log.Printf("No authorization header found, allowing request (auth disabled)")
		return true
	}

	// Parse Bearer token
	if !strings.HasPrefix(authHeader, "Bearer ") {
		log.Printf("Invalid authorization header format")
		return false
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		log.Printf("Empty bearer token")
		return false
	}

	// Validate token (basic validation for now)
	// In a production environment, this should validate against OAuth provider
	if len(token) < 10 {
		log.Printf("Token too short, likely invalid")
		return false
	}

	log.Printf("Token validation passed for token: %s...", token[:10])
	return true
}

// validateOrigin validates the Origin header for security
func (s *Server) validateOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Allow requests without Origin header (for non-browser clients)
		return true
	}

	// Add allowed origins here
	allowedOrigins := []string{
		"https://claude.ai",
		"https://console.anthropic.com",
		"http://localhost:3000", // For development
	}

	for _, allowed := range allowedOrigins {
		if origin == allowed {
			return true
		}
	}

	log.Printf("Origin not allowed: %s", origin)
	return false
}
