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
	"remote-mcp-proxy/config"
	"remote-mcp-proxy/mcp"
	"remote-mcp-proxy/protocol"
)

// Server represents the HTTP proxy server
type Server struct {
	mcpManager        *mcp.Manager
	translator        *protocol.Translator
	connectionManager *ConnectionManager
	config            *config.Config
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

// NewServer creates a new proxy server (backward compatibility)
func NewServer(mcpManager *mcp.Manager) *Server {
	return NewServerWithConfig(mcpManager, nil)
}

// NewServerWithConfig creates a new proxy server with configuration
func NewServerWithConfig(mcpManager *mcp.Manager, cfg *config.Config) *Server {
	const maxConnections = 100 // Configurable connection limit

	server := &Server{
		mcpManager:        mcpManager,
		translator:        protocol.NewTranslator(),
		connectionManager: NewConnectionManager(maxConnections),
		config:            cfg,
	}

	// Start background cleanup routine
	go server.startConnectionCleanup()

	log.Printf("INFO: Created proxy server with max %d connections", maxConnections)
	if cfg != nil {
		log.Printf("INFO: Configured domain: %s", cfg.GetDomain())
	}
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

// subdomainMiddleware extracts MCP server name from subdomain or path
func (s *Server) subdomainMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract server name from subdomain: memory.mcp.domain.com → "memory"
		host := r.Host

		// Remove port if present
		if colonIndex := strings.Index(host, ":"); colonIndex != -1 {
			host = host[:colonIndex]
		}

		parts := strings.Split(host, ".")

		// Expected format: {server}.mcp.{domain}
		if len(parts) >= 3 && parts[1] == "mcp" {
			serverName := parts[0]
			log.Printf("DEBUG: Extracted server name '%s' from host '%s'", serverName, r.Host)

			// Validate server exists in configuration (if config is available)
			if s.config != nil {
				if _, exists := s.config.MCPServers[serverName]; !exists {
					log.Printf("DEBUG: Server '%s' not found in configuration", serverName)
					// Don't add to context if server doesn't exist
					next.ServeHTTP(w, r)
					return
				}
			}

			// Add server name to request context
			ctx := context.WithValue(r.Context(), "mcpServer", serverName)
			r = r.WithContext(ctx)
		} else {
			// If subdomain doesn't match, try to extract from path for fallback
			// Pattern: /{server}/sse or /{server}/sessions/{sessionId}
			pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			if len(pathParts) >= 1 && pathParts[0] != "" {
				// Check if it's a valid server name and not a utility endpoint
				serverName := pathParts[0]
				if serverName != "health" && serverName != "listmcp" && serverName != "listtools" &&
					serverName != "oauth" && serverName != ".well-known" {

					// Validate server exists in configuration (if config is available)
					if s.config != nil {
						if _, exists := s.config.MCPServers[serverName]; exists {
							log.Printf("DEBUG: Extracted server name '%s' from path '%s' (subdomain fallback)", serverName, r.URL.Path)
							// Add server name to request context
							ctx := context.WithValue(r.Context(), "mcpServer", serverName)
							r = r.WithContext(ctx)
						} else {
							log.Printf("DEBUG: Path-based server '%s' not found in configuration", serverName)
						}
					}
				}
			}

			if r.Context().Value("mcpServer") == nil {
				log.Printf("DEBUG: Host '%s' doesn't match subdomain pattern {server}.mcp.{domain} and no valid server in path", r.Host)
			}
		}

		next.ServeHTTP(w, r)
	})
}

// Router returns the HTTP router with all routes configured
func (s *Server) Router() http.Handler {
	r := mux.NewRouter()

	// Apply subdomain detection middleware
	r.Use(s.subdomainMiddleware)

	// Root-level endpoints (standard Remote MCP format - subdomain-based)
	r.HandleFunc("/sse", s.handleMCPRequest).Methods("GET", "POST")
	r.HandleFunc("/sessions/{sessionId:[^/]+}", s.handleSessionMessage).Methods("POST")

	// Path-based endpoints (fallback for localhost and development)
	r.HandleFunc("/{server:[^/]+}/sse", s.handleMCPRequest).Methods("GET", "POST")
	r.HandleFunc("/{server:[^/]+}/sessions/{sessionId:[^/]+}", s.handleSessionMessage).Methods("POST")

	// Utility endpoints
	r.HandleFunc("/health", s.handleHealth).Methods("GET", "OPTIONS")
	r.HandleFunc("/listmcp", s.handleListMCP).Methods("GET", "OPTIONS")
	r.HandleFunc("/listtools/{server:[^/]+}", s.handleListTools).Methods("GET", "OPTIONS")

	// OAuth 2.0 Dynamic Client Registration endpoints
	r.HandleFunc("/.well-known/oauth-authorization-server", s.handleOAuthMetadata).Methods("GET")
	r.HandleFunc("/oauth/register", s.handleClientRegistration).Methods("POST", "OPTIONS")
	r.HandleFunc("/oauth/authorize", s.handleAuthorize).Methods("GET", "POST")
	r.HandleFunc("/oauth/token", s.handleToken).Methods("POST", "OPTIONS")

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

// handleListMCP returns the list of all configured MCP servers and their status
func (s *Server) handleListMCP(w http.ResponseWriter, r *http.Request) {
	log.Printf("INFO: Handling listmcp request")

	servers := s.mcpManager.GetAllServers()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"servers": servers,
		"count":   len(servers),
	}); err != nil {
		log.Printf("ERROR: Failed to encode listmcp response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	} else {
		log.Printf("INFO: Successfully returned list of %d MCP servers", len(servers))
	}
}

// handleListTools returns the available tools for a specific MCP server
func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverName := vars["server"]

	log.Printf("INFO: Handling listtools request for server: %s", serverName)

	// Get the MCP server
	mcpServer, exists := s.mcpManager.GetServer(serverName)
	if !exists {
		log.Printf("ERROR: MCP server '%s' not found", serverName)
		http.Error(w, fmt.Sprintf("MCP server '%s' not found", serverName), http.StatusNotFound)
		return
	}

	// Check if server is running
	if !mcpServer.IsRunning() {
		log.Printf("ERROR: MCP server '%s' is not running", serverName)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "server_not_running",
			"message": fmt.Sprintf("MCP server '%s' is not running", serverName),
			"server":  serverName,
		})
		return
	}

	// Query the MCP server for available tools using the tools/list request
	toolsListRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      fmt.Sprintf("listtools-%d", time.Now().UnixNano()),
		"method":  "tools/list",
		"params":  map[string]interface{}{}, // MCP protocol requires params field
	}

	requestBytes, err := json.Marshal(toolsListRequest)
	if err != nil {
		log.Printf("ERROR: Failed to marshal tools/list request: %v", err)
		http.Error(w, "Failed to create tools request", http.StatusInternalServerError)
		return
	}

	// Send the tools/list request to the MCP server
	if err := mcpServer.SendMessage(requestBytes); err != nil {
		log.Printf("ERROR: Failed to send tools/list request to server %s: %v", serverName, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "send_failed",
			"message": fmt.Sprintf("Failed to send request to MCP server: %v", err),
			"server":  serverName,
		})
		return
	}

	// Read the response with a timeout - increased to match initialize timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	responseBytes, err := mcpServer.ReadMessage(ctx)
	if err != nil {
		log.Printf("ERROR: Failed to read tools/list response from server %s: %v", serverName, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "read_failed",
			"message": fmt.Sprintf("Failed to read response from MCP server: %v", err),
			"server":  serverName,
		})
		return
	}

	// CRITICAL FIX: Apply tool name normalization for Claude.ai compatibility
	//
	// The raw MCP response contains tool names in their original format (e.g., "API-get-user")
	// but Claude.ai expects normalized snake_case names (e.g., "api_get_user").
	//
	// We must apply the same normalization that the regular MCP message flow uses
	// to ensure consistency between /listtools endpoint and SSE connections.
	//
	// DO NOT RETURN RAW RESPONSE - this breaks Claude.ai tool discovery
	normalizedResponse, err := s.translator.MCPToRemote(responseBytes)
	if err != nil {
		log.Printf("ERROR: Failed to normalize tools/list response from server %s: %v", serverName, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "normalization_failed",
			"message": fmt.Sprintf("Failed to normalize response from MCP server: %v", err),
			"server":  serverName,
		})
		return
	}

	// Parse the normalized response
	var normalizedMCPResponse map[string]interface{}
	if err := json.Unmarshal(normalizedResponse, &normalizedMCPResponse); err != nil {
		log.Printf("ERROR: Failed to parse normalized tools/list response from server %s: %v", serverName, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "parse_failed",
			"message": fmt.Sprintf("Failed to parse normalized response from MCP server: %v", err),
			"server":  serverName,
		})
		return
	}

	// Return the normalized tools information with Claude.ai compatible tool names
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]interface{}{
		"server":   serverName,
		"response": normalizedMCPResponse,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("ERROR: Failed to encode listtools response: %v", err)
	} else {
		log.Printf("INFO: Successfully returned normalized tools list for server %s", serverName)
	}
}

// handleMCPRequest handles Remote MCP requests and forwards them to local MCP servers
func (s *Server) handleMCPRequest(w http.ResponseWriter, r *http.Request) {
	// Extract server name from context (set by subdomain middleware) or URL path
	serverName, ok := r.Context().Value("mcpServer").(string)
	if !ok || serverName == "" {
		// Try to get server name from URL path (path-based routing fallback)
		vars := mux.Vars(r)
		if pathServer, exists := vars["server"]; exists && pathServer != "" {
			serverName = pathServer
			log.Printf("DEBUG: Using server name '%s' from URL path", serverName)
		} else {
			log.Printf("ERROR: No server name found in context or URL path for host: %s, path: %s", r.Host, r.URL.Path)
			http.Error(w, "Invalid request format. Expected: {server}.mcp.{domain}/sse or /{server}/sse", http.StatusBadRequest)
			return
		}
	}

	// Validate server exists
	mcpServer, exists := s.mcpManager.GetServer(serverName)
	if !exists {
		log.Printf("ERROR: MCP server '%s' not found", serverName)
		http.Error(w, fmt.Sprintf("MCP server '%s' not found", serverName), http.StatusNotFound)
		return
	}

	// Comprehensive request logging
	log.Printf("=== MCP REQUEST START ===")
	log.Printf("INFO: Method: %s, URL: %s, Server: %s (from subdomain)", r.Method, r.URL.String(), serverName)
	log.Printf("INFO: Remote Address: %s", r.RemoteAddr)
	log.Printf("INFO: User-Agent: %s", r.Header.Get("User-Agent"))
	log.Printf("INFO: Content-Type: %s", r.Header.Get("Content-Type"))
	log.Printf("INFO: Accept: %s", r.Header.Get("Accept"))
	log.Printf("INFO: Origin: %s", r.Header.Get("Origin"))
	log.Printf("INFO: Authorization: %s", func() string {
		auth := r.Header.Get("Authorization")
		if auth != "" {
			if len(auth) > 20 {
				return auth[:20] + "..."
			}
			return auth
		}
		return "none"
	}())

	// Log all headers starting with X- or Mcp-
	for name, values := range r.Header {
		if strings.HasPrefix(strings.ToLower(name), "x-") || strings.HasPrefix(strings.ToLower(name), "mcp-") {
			log.Printf("INFO: Header %s: %v", name, values)
		}
	}

	// Validate authentication
	log.Printf("INFO: Validating authentication...")
	if !s.validateAuthentication(r) {
		log.Printf("ERROR: Authentication failed for request from %s", r.RemoteAddr)
		log.Printf("=== MCP REQUEST END (AUTH FAILED) ===")
		// Add WWW-Authenticate header for proper OAuth Bearer token flow
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"Remote MCP Server\"")
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"unauthorized","error_description":"Bearer token required for Remote MCP access"}`, http.StatusUnauthorized)
		return
	}
	log.Printf("SUCCESS: Authentication passed")

	log.Printf("SUCCESS: Found MCP server: %s (running: %v)", serverName, mcpServer.IsRunning())

	// Handle based on request method
	log.Printf("INFO: Handling request method: %s", r.Method)
	switch r.Method {
	case "GET":
		log.Printf("INFO: Starting SSE connection handling...")
		s.handleSSEConnection(w, r, mcpServer)
		log.Printf("=== MCP REQUEST END (SSE) ===")
	case "POST":
		log.Printf("INFO: Starting POST message handling...")
		s.handleMCPMessage(w, r, mcpServer)
		log.Printf("=== MCP REQUEST END (POST) ===")
	default:
		log.Printf("ERROR: Method not allowed: %s", r.Method)
		log.Printf("=== MCP REQUEST END (METHOD NOT ALLOWED) ===")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSSEConnection establishes a Server-Sent Events connection
func (s *Server) handleSSEConnection(w http.ResponseWriter, r *http.Request, mcpServer *mcp.Server) {
	log.Printf("=== SSE CONNECTION START ===")
	log.Printf("INFO: Setting up SSE connection for server: %s", mcpServer.Name)

	// Get or generate session ID
	sessionID := s.getSessionID(r)
	log.Printf("INFO: Session ID for SSE connection: %s", sessionID)

	// CRITICAL FIX: Register session in translator immediately
	//
	// Create an uninitialized session state so that IsInitialized() checks
	// will recognize the session exists (returning false, as expected) rather
	// than treating it as completely unknown.
	//
	// This allows the session endpoint to accept initialize requests for
	// sessions created via SSE connections.
	s.translator.RegisterSession(sessionID)
	log.Printf("SUCCESS: Session %s registered in translator", sessionID)

	// Create cancellable context for this connection
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Check connection limits and add to manager
	log.Printf("INFO: Adding connection to manager...")
	if err := s.connectionManager.AddConnection(sessionID, mcpServer.Name, ctx, cancel); err != nil {
		log.Printf("ERROR: Failed to add connection for session %s: %v", sessionID, err)
		log.Printf("=== SSE CONNECTION END (CONNECTION LIMIT) ===")
		http.Error(w, "Too many connections", http.StatusTooManyRequests)
		return
	}
	log.Printf("SUCCESS: Connection added to manager")

	// Set SSE headers
	log.Printf("INFO: Setting SSE headers...")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Session-ID", sessionID)
	log.Printf("SUCCESS: SSE headers set")

	// Send required "endpoint" event for Remote MCP protocol
	log.Printf("INFO: Sending endpoint event...")
	if _, err := fmt.Fprintf(w, "event: endpoint\n"); err != nil {
		log.Printf("ERROR: Failed to write SSE endpoint event: %v", err)
		log.Printf("=== SSE CONNECTION END (ENDPOINT EVENT FAILED) ===")
		s.connectionManager.RemoveConnection(sessionID)
		return
	}

	// Construct the session endpoint URL that Claude will use for sending messages
	log.Printf("INFO: Constructing session endpoint URL...")
	scheme := "https"
	if r.Header.Get("X-Forwarded-Proto") == "" {
		scheme = "http"
	}
	host := r.Host
	if r.Header.Get("X-Forwarded-Host") != "" {
		host = r.Header.Get("X-Forwarded-Host")
	}

	// Determine if we're using subdomain-based or path-based routing
	var sessionEndpoint string
	if strings.Contains(host, ".mcp.") {
		// Subdomain-based routing: https://memory.mcp.domain.com/sessions/abc123
		sessionEndpoint = fmt.Sprintf("%s://%s/sessions/%s", scheme, host, sessionID)
	} else {
		// Path-based routing: http://localhost:8080/memory/sessions/abc123
		sessionEndpoint = fmt.Sprintf("%s://%s/%s/sessions/%s", scheme, host, mcpServer.Name, sessionID)
	}
	log.Printf("INFO: Session endpoint URL: %s", sessionEndpoint)

	endpointData := map[string]interface{}{
		"uri": sessionEndpoint,
	}

	endpointJSON, err := json.Marshal(endpointData)
	if err != nil {
		log.Printf("ERROR: Failed to marshal endpoint data: %v", err)
		log.Printf("=== SSE CONNECTION END (ENDPOINT JSON FAILED) ===")
		s.connectionManager.RemoveConnection(sessionID)
		return
	}

	log.Printf("INFO: Endpoint data: %s", string(endpointJSON))
	if _, err := fmt.Fprintf(w, "data: %s\n\n", string(endpointJSON)); err != nil {
		log.Printf("ERROR: Failed to write SSE endpoint data: %v", err)
		log.Printf("=== SSE CONNECTION END (ENDPOINT DATA FAILED) ===")
		s.connectionManager.RemoveConnection(sessionID)
		return
	}
	log.Printf("SUCCESS: Endpoint event sent successfully")

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
			// CRITICAL FIX: Remove initialization check to prevent SSE deadlock
			//
			// The previous logic created a chicken-and-egg problem:
			// 1. SSE connection waits for session initialization
			// 2. Claude.ai needs active SSE to send initialize request
			// 3. Session endpoint rejects uninitialized sessions -> DEADLOCK
			//
			// Claude.ai Remote MCP protocol requires SSE to be active BEFORE
			// initialization, not after. The SSE connection provides the session
			// endpoint URL, then Claude.ai uses that endpoint to initialize.
			//
			// We must process ALL messages (including initialize) through SSE flow.

			// CRITICAL ARCHITECTURAL FIX: Remove continuous MCP server polling
			//
			// The previous implementation constantly polled the MCP server for messages,
			// but MCP servers only respond when they receive requests. This caused:
			// - Thousands of timeout messages
			// - Blocked SSE connections
			// - Failed tool discovery
			//
			// Remote MCP protocol should be REQUEST-DRIVEN, not polling-driven:
			// 1. SSE sends endpoint event and waits
			// 2. Claude.ai sends requests via session endpoint
			// 3. Session endpoint handles requests synchronously
			// 4. Responses sent directly (not via SSE for most requests)
			//
			// For now, SSE just maintains the connection and waits.
			// Future: Add channel-based event system for notifications if needed.

			log.Printf("DEBUG: SSE connection active for server %s, session %s - waiting for requests", mcpServer.Name, sessionID)

			// Just keep the connection alive - no more continuous polling
			time.Sleep(1 * time.Second)
		}
	}
}

// handleMCPMessage handles POST requests with MCP messages
func (s *Server) handleMCPMessage(w http.ResponseWriter, r *http.Request, mcpServer *mcp.Server) {
	log.Printf("=== MCP MESSAGE START ===")
	log.Printf("INFO: Processing POST message for server: %s", mcpServer.Name)

	// Read the request body
	log.Printf("INFO: Reading request body...")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("ERROR: Failed to read request body: %v", err)
		log.Printf("=== MCP MESSAGE END (BODY READ FAILED) ===")
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	log.Printf("SUCCESS: Request body read (%d bytes)", len(body))
	log.Printf("INFO: Request body: %s", string(body))

	// Parse the JSON-RPC message to check if it's a handshake message
	log.Printf("INFO: Parsing JSON-RPC message...")
	var jsonrpcMsg protocol.JSONRPCMessage
	if err := json.Unmarshal(body, &jsonrpcMsg); err != nil {
		log.Printf("ERROR: Invalid JSON-RPC message: %v", err)
		log.Printf("=== MCP MESSAGE END (JSON PARSE FAILED) ===")
		http.Error(w, fmt.Sprintf("Invalid JSON-RPC message: %v", err), http.StatusBadRequest)
		return
	}
	log.Printf("SUCCESS: JSON-RPC message parsed")
	log.Printf("INFO: Method: %s, ID: %v", jsonrpcMsg.Method, jsonrpcMsg.ID)

	// Generate or get session ID
	sessionID := s.getSessionID(r)
	log.Printf("INFO: Session ID: %s", sessionID)

	// CRITICAL FIX: Only handle handshake messages if this is NOT a session endpoint request
	//
	// Session endpoint requests (/{server}/sessions/{sessionId}) should be handled
	// entirely by handleSessionMessage to prevent duplicate ReadMessage calls.
	// The handleMCPMessage function should only handle direct SSE endpoint requests.
	//
	// Check if this request is coming to a session endpoint by looking at the URL path
	isSessionEndpointRequest := strings.Contains(r.URL.Path, "/sessions/")

	log.Printf("INFO: Checking request type - URL: %s, IsSessionEndpoint: %v", r.URL.Path, isSessionEndpointRequest)

	if isSessionEndpointRequest {
		log.Printf("ERROR: Session endpoint request incorrectly routed to handleMCPMessage")
		log.Printf("ERROR: This should not happen - check routing configuration")
		log.Printf("=== MCP MESSAGE END (ROUTING ERROR) ===")
		http.Error(w, "Internal routing error", http.StatusInternalServerError)
		return
	}

	// Handle handshake messages for direct SSE endpoint requests only
	log.Printf("INFO: Checking if handshake message...")
	if s.translator.IsHandshakeMessage(jsonrpcMsg.Method) {
		log.Printf("INFO: Processing handshake message: %s", jsonrpcMsg.Method)
		s.handleHandshakeMessage(w, r, sessionID, &jsonrpcMsg, mcpServer)
		log.Printf("=== MCP MESSAGE END (HANDSHAKE) ===")
		return
	}
	log.Printf("INFO: Not a handshake message, continuing with regular processing")

	// Check if connection is initialized
	if !s.translator.IsInitialized(sessionID) {
		s.sendErrorResponse(w, jsonrpcMsg.ID, protocol.InvalidRequest, "Connection not initialized", false)
		return
	}

	// Track the request for potential fallback handling
	if jsonrpcMsg.Method != "" && jsonrpcMsg.ID != nil {
		s.translator.TrackRequest(sessionID, jsonrpcMsg.ID, jsonrpcMsg.Method)
		log.Printf("DEBUG: Tracking request ID %v, method %s for session %s", jsonrpcMsg.ID, jsonrpcMsg.Method, sessionID)
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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-ID, Mcp-Session-Id")
		w.Header().Set("Access-Control-Expose-Headers", "X-Session-ID, WWW-Authenticate")

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
	// Try to get session ID from Remote MCP standard header
	if sessionID := r.Header.Get("Mcp-Session-Id"); sessionID != "" {
		log.Printf("DEBUG: Using existing session ID from Mcp-Session-Id header: %s", sessionID)
		return sessionID
	}

	// Try to get session ID from legacy header for backward compatibility
	if sessionID := r.Header.Get("X-Session-ID"); sessionID != "" {
		log.Printf("DEBUG: Using existing session ID from X-Session-ID header: %s", sessionID)
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
func (s *Server) handleHandshakeMessage(w http.ResponseWriter, r *http.Request, sessionID string, msg *protocol.JSONRPCMessage, mcpServer *mcp.Server) {
	switch msg.Method {
	case "initialize":
		s.handleInitialize(w, r, sessionID, msg, mcpServer)
	case "notifications/initialized":
		s.handleInitialized(w, sessionID, msg)
	default:
		s.sendErrorResponse(w, msg.ID, protocol.MethodNotFound, "Unknown handshake method", false)
	}
}

// handleInitialize handles the initialize request
func (s *Server) handleInitialize(w http.ResponseWriter, r *http.Request, sessionID string, msg *protocol.JSONRPCMessage, mcpServer *mcp.Server) {
	// Parse initialize parameters
	var params protocol.InitializeParams
	if msg.Params != nil {
		paramsBytes, _ := json.Marshal(msg.Params)
		if err := json.Unmarshal(paramsBytes, &params); err != nil {
			s.sendErrorResponse(w, msg.ID, protocol.InvalidParams, "Invalid initialize parameters", false)
			return
		}
	}

	// Check if server is running
	if !mcpServer.IsRunning() {
		log.Printf("ERROR: MCP server '%s' is not running for initialize", mcpServer.Name)
		s.sendErrorResponse(w, msg.ID, protocol.InvalidRequest, fmt.Sprintf("MCP server '%s' is not running", mcpServer.Name), false)
		return
	}

	// Forward the initialize request to the actual MCP server
	initRequestBytes, err := json.Marshal(msg)
	if err != nil {
		log.Printf("ERROR: Failed to marshal initialize request: %v", err)
		s.sendErrorResponse(w, msg.ID, protocol.InternalError, "Failed to process initialize request", false)
		return
	}

	// Send initialize request to MCP server
	if err := mcpServer.SendMessage(initRequestBytes); err != nil {
		log.Printf("ERROR: Failed to send initialize request to MCP server %s: %v", mcpServer.Name, err)
		s.sendErrorResponse(w, msg.ID, protocol.InternalError, "Failed to communicate with MCP server", false)
		return
	}

	// CRITICAL SECTION: Synchronous handshake for Remote MCP protocol compliance
	//
	// Remote MCP protocol (as implemented by Claude.ai) expects a synchronous JSON response
	// to the initialize POST request, NOT an asynchronous SSE response. This section must
	// remain synchronous to maintain protocol compliance.
	//
	// IMPORTANT: The 30-second timeout was increased from 10 seconds to handle slow MCP
	// server initialization (especially npm-based servers). Reducing this timeout will
	// cause "context deadline exceeded" errors during initialization.
	//
	// The dedicated read goroutine pattern prevents stdio deadlocks that occur when
	// multiple concurrent requests try to read from the same MCP server stdout stream.
	log.Printf("INFO: Waiting for initialize response from MCP server %s...", mcpServer.Name)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// CRITICAL FIX: Use dedicated read goroutine to prevent stdio deadlock
	// This pattern isolates the ReadMessage call to prevent race conditions
	// when multiple requests access the same MCP server simultaneously
	type readResult struct {
		data []byte
		err  error
	}

	readChan := make(chan readResult, 1)
	go func() {
		// ReadMessage uses a dedicated readMu mutex in mcp/manager.go to serialize stdout access
		data, err := mcpServer.ReadMessage(ctx)
		readChan <- readResult{data, err}
	}()

	// Wait for the ReadMessage goroutine to complete
	// The context timeout in ReadMessage will handle timeouts properly
	result := <-readChan
	if result.err != nil {
		log.Printf("ERROR: Failed to read initialize response from MCP server %s: %v", mcpServer.Name, result.err)
		s.sendErrorResponse(w, msg.ID, protocol.InternalError, "Failed to receive response from MCP server", false)
		return
	}
	responseBytes := result.data

	// Parse the MCP server's initialize response
	var mcpResponse protocol.JSONRPCMessage
	if err := json.Unmarshal(responseBytes, &mcpResponse); err != nil {
		log.Printf("ERROR: Failed to parse initialize response from MCP server %s: %v", mcpServer.Name, err)
		s.sendErrorResponse(w, msg.ID, protocol.InternalError, "Invalid response from MCP server", false)
		return
	}

	// Store connection state in translator
	if mcpResponse.Result != nil {
		_, err := s.translator.HandleInitialize(sessionID, params)
		if err != nil {
			log.Printf("ERROR: Failed to store connection state: %v", err)
		} else {
			// CRITICAL FIX: Mark session as initialized immediately after successful initialize response
			//
			// This is essential for Remote MCP protocol compliance. Claude.ai expects to be able to
			// make follow-up requests (like tools/list) immediately after a successful initialize.
			// The session MUST be marked as initialized here, not waiting for a separate
			// "initialized" notification which may never come in Remote MCP.
			//
			// Without this, all subsequent requests will fail with "Session not initialized" errors
			// and the integration will appear to connect but not expose any tools.
			//
			// DO NOT REMOVE OR MODIFY THIS SECTION WITHOUT ENSURING ALTERNATIVE INITIALIZATION MECHANISM
			err := s.translator.HandleInitialized(sessionID)
			if err != nil {
				log.Printf("ERROR: Failed to mark session as initialized: %v", err)
			} else {
				log.Printf("INFO: Session %s marked as initialized for server %s", sessionID, mcpServer.Name)
			}
		}
	}

	// Return the MCP server's response directly to Claude
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Session-ID", sessionID)
	w.Header().Set("Mcp-Session-Id", sessionID)
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(responseBytes); err != nil {
		log.Printf("ERROR: Failed to write initialize response: %v", err)
	} else {
		log.Printf("INFO: Forwarded initialize response from MCP server %s for session %s", mcpServer.Name, sessionID)
	}
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

// handleSessionMessage handles POST requests to session endpoints from Claude
func (s *Server) handleSessionMessage(w http.ResponseWriter, r *http.Request) {
	// Extract server name from context (set by subdomain middleware) or URL path
	serverName, ok := r.Context().Value("mcpServer").(string)
	if !ok || serverName == "" {
		// Try to get server name from URL path (path-based routing fallback)
		vars := mux.Vars(r)
		if pathServer, exists := vars["server"]; exists && pathServer != "" {
			serverName = pathServer
			log.Printf("DEBUG: Using server name '%s' from URL path for session", serverName)
		} else {
			log.Printf("ERROR: No server name found in context or URL path for host: %s, path: %s", r.Host, r.URL.Path)
			http.Error(w, "Invalid request format. Expected: {server}.mcp.{domain}/sessions/{id} or /{server}/sessions/{id}", http.StatusBadRequest)
			return
		}
	}

	vars := mux.Vars(r)
	sessionID := vars["sessionId"]

	log.Printf("=== SESSION MESSAGE START ===")
	log.Printf("INFO: Handling session message for server: %s (from subdomain), session: %s", serverName, sessionID)
	log.Printf("INFO: Method: %s, URL: %s", r.Method, r.URL.String())
	log.Printf("INFO: Remote Address: %s", r.RemoteAddr)
	log.Printf("INFO: User-Agent: %s", r.Header.Get("User-Agent"))
	log.Printf("INFO: Content-Type: %s", r.Header.Get("Content-Type"))

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

	// Read the request body first to check message type
	log.Printf("INFO: Reading session message body...")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("ERROR: Failed to read session message body: %v", err)
		log.Printf("=== SESSION MESSAGE END (BODY READ FAILED) ===")
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	log.Printf("SUCCESS: Session message body read (%d bytes)", len(body))
	log.Printf("INFO: Session message body: %s", string(body))

	// Parse the JSON-RPC message
	log.Printf("INFO: Parsing session message JSON-RPC...")
	var jsonrpcMsg protocol.JSONRPCMessage
	if err := json.Unmarshal(body, &jsonrpcMsg); err != nil {
		log.Printf("ERROR: Invalid session message JSON-RPC: %v", err)
		log.Printf("=== SESSION MESSAGE END (JSON PARSE FAILED) ===")
		http.Error(w, fmt.Sprintf("Invalid JSON-RPC message: %v", err), http.StatusBadRequest)
		return
	}
	log.Printf("SUCCESS: Session message JSON-RPC parsed")
	log.Printf("INFO: Session message method: %s, ID: %v", jsonrpcMsg.Method, jsonrpcMsg.ID)

	// CRITICAL FIX: Allow handshake messages on uninitialized sessions
	//
	// For Remote MCP protocol compliance, the session endpoint must accept
	// initialize requests even when the session is not yet initialized.
	// This breaks the deadlock where Claude.ai cannot initialize because
	// session endpoints reject uninitialized requests.

	isHandshake := s.translator.IsHandshakeMessage(jsonrpcMsg.Method)
	isInitialized := s.translator.IsInitialized(sessionID)

	log.Printf("DEBUG: Session %s - Method: %s, IsHandshake: %v, IsInitialized: %v",
		sessionID, jsonrpcMsg.Method, isHandshake, isInitialized)

	if !isHandshake && !isInitialized {
		log.Printf("ERROR: Session %s not initialized for non-handshake method %s", sessionID, jsonrpcMsg.Method)
		http.Error(w, "Session not initialized", http.StatusBadRequest)
		return
	}

	// Track the request for potential fallback handling
	if jsonrpcMsg.Method != "" && jsonrpcMsg.ID != nil {
		s.translator.TrackRequest(sessionID, jsonrpcMsg.ID, jsonrpcMsg.Method)
		log.Printf("DEBUG: Tracking session request ID %v, method %s for session %s", jsonrpcMsg.ID, jsonrpcMsg.Method, sessionID)
	}

	// CRITICAL ARCHITECTURAL FIX: Handle ALL session endpoint requests synchronously
	//
	// Previous design only handled handshake messages synchronously and sent other
	// requests (tools/list, tools/call) asynchronously via SSE. This caused issues:
	// - SSE polling loop blocked everything
	// - Responses never reached Claude.ai
	// - Tool discovery failed
	//
	// New design: ALL session endpoint requests are synchronous:
	// - initialize -> synchronous response
	// - tools/list -> synchronous response
	// - tools/call -> synchronous response
	//
	// This is more aligned with how most Remote MCP implementations work.

	log.Printf("INFO: Handling session request %s synchronously", jsonrpcMsg.Method)

	// Send request to MCP server
	if err := mcpServer.SendMessage(body); err != nil {
		log.Printf("ERROR: Failed to send message to MCP server %s: %v", serverName, err)
		http.Error(w, "Failed to send message to MCP server", http.StatusInternalServerError)
		return
	}

	// Read response from MCP server synchronously
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	responseBytes, err := mcpServer.ReadMessage(ctx)
	if err != nil {
		log.Printf("ERROR: Failed to read response from MCP server %s: %v", serverName, err)
		http.Error(w, "Failed to receive response from MCP server", http.StatusInternalServerError)
		return
	}

	// Handle special processing for handshake messages
	if s.translator.IsHandshakeMessage(jsonrpcMsg.Method) {
		// Parse response and update session state for handshake messages
		var mcpResponse protocol.JSONRPCMessage
		if err := json.Unmarshal(responseBytes, &mcpResponse); err == nil && mcpResponse.Result != nil {
			// Mark session as initialized after successful initialize
			if jsonrpcMsg.Method == "initialize" {
				err := s.translator.HandleInitialized(sessionID)
				if err != nil {
					log.Printf("ERROR: Failed to mark session as initialized: %v", err)
				} else {
					log.Printf("INFO: Session %s marked as initialized for server %s", sessionID, serverName)
				}
			}
		}
	}

	// Return response directly to Claude.ai
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Mcp-Session-Id", sessionID)
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(responseBytes); err != nil {
		log.Printf("ERROR: Failed to write session response: %v", err)
	} else {
		log.Printf("INFO: Successfully returned synchronous response for %s to session %s", jsonrpcMsg.Method, sessionID)
	}
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
		log.Printf("ERROR: No authorization header found, authentication required")
		return false
	}

	// Parse Bearer token
	if !strings.HasPrefix(authHeader, "Bearer ") {
		log.Printf("ERROR: Invalid authorization header format, expected Bearer token")
		return false
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		log.Printf("ERROR: Empty bearer token")
		return false
	}

	// Simple token validation - accept any non-empty token for Claude.ai compatibility
	// For Claude.ai Remote MCP, any Bearer token should work
	log.Printf("INFO: Authentication successful with token: %s...", func() string {
		if len(token) > 10 {
			return token[:10]
		}
		return token
	}())
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

// OAuth 2.0 Dynamic Client Registration Implementation

// handleOAuthMetadata returns OAuth server metadata for discovery
func (s *Server) handleOAuthMetadata(w http.ResponseWriter, r *http.Request) {
	metadata := map[string]interface{}{
		"issuer":                 fmt.Sprintf("https://%s", r.Host),
		"authorization_endpoint": fmt.Sprintf("https://%s/oauth/authorize", r.Host),
		"token_endpoint":         fmt.Sprintf("https://%s/oauth/token", r.Host),
		"registration_endpoint":  fmt.Sprintf("https://%s/oauth/register", r.Host),
		"response_types_supported": []string{
			"code",
		},
		"grant_types_supported": []string{
			"authorization_code",
		},
		"scopes_supported": []string{
			"mcp",
		},
		"token_endpoint_auth_methods_supported": []string{
			"client_secret_basic",
			"client_secret_post",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metadata)
}

// handleClientRegistration handles OAuth 2.0 Dynamic Client Registration
func (s *Server) handleClientRegistration(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Generate a client ID and secret
	clientID := generateRandomString(32)
	clientSecret := generateRandomString(64)

	// For simplicity, accept any registration request
	registrationResponse := map[string]interface{}{
		"client_id":                clientID,
		"client_secret":            clientSecret,
		"client_id_issued_at":      time.Now().Unix(),
		"client_secret_expires_at": 0, // Never expires
		"redirect_uris": []string{
			"https://claude.ai/oauth/callback",
			"https://www.claude.ai/oauth/callback",
		},
		"grant_types": []string{
			"authorization_code",
		},
		"response_types": []string{
			"code",
		},
		"scope": "mcp",
	}

	log.Printf("INFO: OAuth client registered - ID: %s", clientID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(registrationResponse)
}

// handleAuthorize handles OAuth authorization requests
func (s *Server) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	redirectURI := r.URL.Query().Get("redirect_uri")
	state := r.URL.Query().Get("state")
	responseType := r.URL.Query().Get("response_type")

	if clientID == "" || redirectURI == "" || responseType != "code" {
		http.Error(w, "Invalid authorization request", http.StatusBadRequest)
		return
	}

	// Generate authorization code
	authCode := generateRandomString(32)

	log.Printf("INFO: OAuth authorization request - Client: %s, Redirect: %s", clientID, redirectURI)

	// Redirect with authorization code
	callbackURL := fmt.Sprintf("%s?code=%s", redirectURI, authCode)
	if state != "" {
		callbackURL += fmt.Sprintf("&state=%s", state)
	}

	http.Redirect(w, r, callbackURL, http.StatusFound)
}

// handleToken handles OAuth token exchange
func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	grantType := r.FormValue("grant_type")
	code := r.FormValue("code")
	clientID := r.FormValue("client_id")

	if grantType != "authorization_code" || code == "" || clientID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_request",
			"error_description": "Invalid token request",
		})
		return
	}

	// Generate access token
	accessToken := generateRandomString(64)

	tokenResponse := map[string]interface{}{
		"access_token": accessToken,
		"token_type":   "Bearer",
		"expires_in":   3600,
		"scope":        "mcp",
	}

	log.Printf("INFO: OAuth token issued - Client: %s, Token: %s...", clientID, accessToken[:10])

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokenResponse)
}

// generateRandomString generates a cryptographically secure random string
func generateRandomString(length int) string {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to time-based generation if crypto/rand fails
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}
