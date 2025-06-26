package proxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"remote-mcp-proxy/config"
	"remote-mcp-proxy/health"
	"remote-mcp-proxy/logger"
	"remote-mcp-proxy/mcp"
	"remote-mcp-proxy/monitoring"
	"remote-mcp-proxy/protocol"
)

// Server represents the HTTP proxy server
type Server struct {
	mcpManager        *mcp.Manager
	translator        *protocol.Translator
	connectionManager *ConnectionManager
	config            *config.Config
	healthChecker     *health.HealthChecker
	resourceMonitor   *monitoring.ResourceMonitor
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
		logger.System().Warn(" Connection limit reached (%d), rejecting new connection for session %s", cm.maxConnections, sessionID)
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

	logger.System().Info("Added connection for session %s (total: %d/%d)", sessionID, len(cm.connections), cm.maxConnections)
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
		logger.System().Info("Removed connection for session %s (remaining: %d)", sessionID, len(cm.connections))
	}
}

// GetConnectionCount returns the current number of active connections
func (cm *ConnectionManager) GetConnectionCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return len(cm.connections)
}

// GetConnections returns a copy of all active connections
func (cm *ConnectionManager) GetConnections() map[string]*ConnectionInfo {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Create a copy to avoid race conditions
	connections := make(map[string]*ConnectionInfo)
	for sessionID, conn := range cm.connections {
		connections[sessionID] = conn
	}
	return connections
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
		logger.System().Info("Cleaned up %d stale connections: %v", len(removed), removed)
	}
}

// NewServer creates a new proxy server (backward compatibility)
func NewServer(mcpManager *mcp.Manager) *Server {
	return NewServerWithConfig(mcpManager, nil, nil, nil)
}

// NewServerWithConfig creates a new proxy server with configuration
func NewServerWithConfig(mcpManager *mcp.Manager, cfg *config.Config, healthChecker *health.HealthChecker, resourceMonitor *monitoring.ResourceMonitor) *Server {
	const maxConnections = 100 // Configurable connection limit

	server := &Server{
		mcpManager:        mcpManager,
		translator:        protocol.NewTranslator(),
		connectionManager: NewConnectionManager(maxConnections),
		config:            cfg,
		healthChecker:     healthChecker,
		resourceMonitor:   resourceMonitor,
	}

	// Start background cleanup routine
	go server.startConnectionCleanup()

	logger.System().Info("Created proxy server with max %d connections", maxConnections)
	if cfg != nil {
		logger.System().Info("Configured domain: %s", cfg.GetDomain())
	}
	return server
}

// startConnectionCleanup starts a background goroutine to clean up stale connections
func (s *Server) startConnectionCleanup() {
	ticker := time.NewTicker(30 * time.Second) // Cleanup every 30 seconds
	defer ticker.Stop()

	maxAge := 2 * time.Minute // Remove connections older than 2 minutes for faster cleanup

	logger.System().Info("Started automatic connection cleanup (interval: 30s, max age: %v)", maxAge)

	for {
		select {
		case <-ticker.C:
			beforeCount := s.connectionManager.GetConnectionCount()
			s.connectionManager.CleanupStaleConnections(maxAge)
			afterCount := s.connectionManager.GetConnectionCount()

			if beforeCount != afterCount {
				logger.System().Info("Automatic cleanup removed %d stale connections (%d -> %d active)",
					beforeCount-afterCount, beforeCount, afterCount)
			}
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
			logger.System().Debug(" Extracted server name '%s' from host '%s'", serverName, r.Host)

			// Validate server exists in configuration (if config is available)
			if s.config != nil {
				if _, exists := s.config.MCPServers[serverName]; !exists {
					logger.System().Debug(" Server '%s' not found in configuration", serverName)
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
					serverName != "cleanup" && serverName != "oauth" && serverName != ".well-known" {

					// Validate server exists in configuration (if config is available)
					if s.config != nil {
						if _, exists := s.config.MCPServers[serverName]; exists {
							logger.System().Debug(" Extracted server name '%s' from path '%s' (subdomain fallback)", serverName, r.URL.Path)
							// Add server name to request context
							ctx := context.WithValue(r.Context(), "mcpServer", serverName)
							r = r.WithContext(ctx)
						} else {
							logger.System().Debug(" Path-based server '%s' not found in configuration", serverName)
						}
					}
				}
			}

			if r.Context().Value("mcpServer") == nil {
				logger.System().Debug(" Host '%s' doesn't match subdomain pattern {server}.mcp.{domain} and no valid server in path", r.Host)
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
	r.HandleFunc("/cleanup", s.handleCleanup).Methods("POST", "OPTIONS")

	// Health and monitoring endpoints
	r.HandleFunc("/health/servers", s.handleServerHealth).Methods("GET", "OPTIONS")
	r.HandleFunc("/health/resources", s.handleResourceMetrics).Methods("GET", "OPTIONS")
	r.HandleFunc("/health/sessions", s.handleSessionHealth).Methods("GET", "OPTIONS")
	r.HandleFunc("/health/sessions/{sessionId:[^/]+}", s.handleSessionDetail).Methods("GET", "OPTIONS")

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
		logger.System().Error(" Failed to write health response: %v", err)
	} else {
		logger.System().Debug(" Health check response sent successfully")
	}
}

// handleListMCP returns the list of all configured MCP servers and their status
func (s *Server) handleListMCP(w http.ResponseWriter, r *http.Request) {
	logger.System().Info("Handling listmcp request")

	servers := s.mcpManager.GetAllServers()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"servers": servers,
		"count":   len(servers),
	}); err != nil {
		logger.System().Error(" Failed to encode listmcp response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	} else {
		logger.System().Info("Successfully returned list of %d MCP servers", len(servers))
	}
}

// handleCleanup manually cleans up stale connections and sessions
func (s *Server) handleCleanup(w http.ResponseWriter, r *http.Request) {
	logger.System().Info("Handling manual cleanup request")

	// Get current connections before cleanup
	connectionsBefore := s.connectionManager.GetConnections()
	countBefore := len(connectionsBefore)

	// Force cleanup of all stale connections (older than 1 second for immediate cleanup)
	s.connectionManager.CleanupStaleConnections(1 * time.Second)

	// Get connections after cleanup
	connectionsAfter := s.connectionManager.GetConnections()
	countAfter := len(connectionsAfter)

	cleanedCount := countBefore - countAfter

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]interface{}{
		"message":            "Cleanup completed",
		"connections_before": countBefore,
		"connections_after":  countAfter,
		"cleaned_count":      cleanedCount,
		"remaining_sessions": make([]map[string]interface{}, 0),
	}

	// Add details about remaining connections
	for sessionID, conn := range connectionsAfter {
		response["remaining_sessions"] = append(response["remaining_sessions"].([]map[string]interface{}), map[string]interface{}{
			"session_id":   sessionID,
			"server_name":  conn.ServerName,
			"connected_at": conn.ConnectedAt,
		})
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.System().Error(" Failed to encode cleanup response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	} else {
		logger.System().Info("Manual cleanup completed - cleaned %d connections, %d remaining", cleanedCount, countAfter)
	}
}

// handleSessionHealth returns information about all active sessions
func (s *Server) handleSessionHealth(w http.ResponseWriter, r *http.Request) {
	logger.System().Info("Handling session health request")

	// Get active connections which represent active sessions
	connections := s.connectionManager.GetConnections()
	
	// Build session summary
	sessions := make(map[string]interface{})
	
	for sessionID, conn := range connections {
		// Get session-specific server information
		sessionServers := s.mcpManager.GetSessionServers(sessionID)
		
		sessions[sessionID[:8]] = map[string]interface{}{
			"sessionId":     sessionID[:8],
			"fullSessionId": sessionID,
			"serverName":    conn.ServerName,
			"connectedAt":   conn.ConnectedAt,
			"duration":      time.Since(conn.ConnectedAt).String(),
			"servers":       sessionServers,
			"serverCount":   len(sessionServers),
		}
	}

	response := map[string]interface{}{
		"sessions":    sessions,
		"totalSessions": len(sessions),
		"timestamp":   time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.System().Error("Failed to encode session health response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	} else {
		logger.System().Info("Successfully returned session health for %d sessions", len(sessions))
	}
}

// handleSessionDetail returns detailed information about a specific session
func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["sessionId"]

	logger.System().Info("Handling session detail request for session: %s", sessionID)

	// Find the full session ID (we store shortened versions in the URL)
	connections := s.connectionManager.GetConnections()
	var fullSessionID string
	var connection *ConnectionInfo
	
	for fullID, conn := range connections {
		if strings.HasPrefix(fullID, sessionID) {
			fullSessionID = fullID
			connection = conn
			break
		}
	}

	if connection == nil {
		logger.System().Error("Session '%s' not found", sessionID)
		http.Error(w, fmt.Sprintf("Session '%s' not found", sessionID), http.StatusNotFound)
		return
	}

	// Get session-specific server information
	sessionServers := s.mcpManager.GetSessionServers(fullSessionID)
	
	response := map[string]interface{}{
		"sessionId":     sessionID,
		"fullSessionId": fullSessionID,
		"serverName":    connection.ServerName,
		"connectedAt":   connection.ConnectedAt,
		"duration":      time.Since(connection.ConnectedAt).String(),
		"servers":       sessionServers,
		"serverCount":   len(sessionServers),
		"sessionDirectory": fmt.Sprintf("/app/sessions/%s", fullSessionID),
		"timestamp":     time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.System().Error("Failed to encode session detail response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	} else {
		logger.System().Info("Successfully returned session detail for session %s", sessionID)
	}
}

// handleListTools returns the available tools for a specific MCP server
func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverName := vars["server"]

	logger.System().Info("Handling listtools request for server: %s", serverName)

	// Get session ID for session-aware server selection
	sessionID := s.getSessionID(r)
	logger.System().Debug("Using session ID: %s for listtools", sessionID[:8])

	// Get the session-aware MCP server
	mcpServer, exists := s.mcpManager.GetServerForSession(sessionID, serverName)
	if !exists {
		logger.System().Error(" MCP server '%s' not found or failed to create for session %s", serverName, sessionID[:8])
		http.Error(w, fmt.Sprintf("MCP server '%s' not available", serverName), http.StatusNotFound)
		return
	}

	// Check if server is running
	if !mcpServer.IsRunning() {
		logger.System().Error(" MCP server '%s' is not running", serverName)
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
		logger.System().Error(" Failed to marshal tools/list request: %v", err)
		http.Error(w, "Failed to create tools request", http.StatusInternalServerError)
		return
	}

	// Send the tools/list request and receive response using serialized queue
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	responseBytes, err := mcpServer.SendAndReceive(ctx, requestBytes)
	if err != nil {
		logger.System().Error(" Failed to send/receive tools/list request to server %s: %v", serverName, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "request_failed",
			"message": fmt.Sprintf("Failed to communicate with MCP server: %v", err),
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
		logger.System().Error(" Failed to normalize tools/list response from server %s: %v", serverName, err)
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
		logger.System().Error(" Failed to parse normalized tools/list response from server %s: %v", serverName, err)
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
		logger.System().Error(" Failed to encode listtools response: %v", err)
	} else {
		logger.System().Info("Successfully returned normalized tools list for server %s", serverName)
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
			logger.System().Debug(" Using server name '%s' from URL path", serverName)
		} else {
			logger.System().Error(" No server name found in context or URL path for host: %s, path: %s", r.Host, r.URL.Path)
			http.Error(w, "Invalid request format. Expected: {server}.mcp.{domain}/sse or /{server}/sse", http.StatusBadRequest)
			return
		}
	}

	// Get session ID early for session-aware server selection
	sessionID := s.getSessionID(r)
	logger.System().Debug("Using session ID: %s for server selection", sessionID[:8])

	// Use session-aware server selection
	mcpServer, exists := s.mcpManager.GetServerForSession(sessionID, serverName)
	if !exists {
		logger.System().Error(" MCP server '%s' not found or failed to create for session %s", serverName, sessionID[:8])
		http.Error(w, fmt.Sprintf("MCP server '%s' not available", serverName), http.StatusNotFound)
		return
	}

	// Consolidated request logging
	logger.System().Debug(">>> MCP %s %s via %s", r.Method, r.URL.String(), serverName)

	// Auth and session logging moved to TRACE level
	if auth := r.Header.Get("Authorization"); auth != "" {
		logger.System().Trace("Auth: %s", func() string {
			if len(auth) > 15 {
				return auth[:15] + "..."
			}
			return auth
		}())
	}

	if sessionID := r.Header.Get("Mcp-Session-Id"); sessionID != "" {
		logger.System().Trace("Session: %s", sessionID)
	}

	// Validate authentication
	logger.System().Info("Validating authentication...")
	if !s.validateAuthentication(r) {
		logger.System().Error(" Authentication failed for request from %s", r.RemoteAddr)
		logger.System().Info("=== MCP REQUEST END (AUTH FAILED) ===")
		// Add WWW-Authenticate header for proper OAuth Bearer token flow
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"Remote MCP Server\"")
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"unauthorized","error_description":"Bearer token required for Remote MCP access"}`, http.StatusUnauthorized)
		return
	}
	logger.System().Info("SUCCESS: Authentication passed")

	logger.System().Info("SUCCESS: Found MCP server: %s (running: %v)", serverName, mcpServer.IsRunning())

	// Handle based on request method
	logger.System().Info("Handling request method: %s", r.Method)
	switch r.Method {
	case "GET":
		logger.System().Info("Starting SSE connection handling...")
		s.handleSSEConnection(w, r, mcpServer)
		logger.System().Info("=== MCP REQUEST END (SSE) ===")
	case "POST":
		logger.System().Info("Starting POST message handling...")
		s.handleMCPMessage(w, r, mcpServer)
		logger.System().Info("=== MCP REQUEST END (POST) ===")
	default:
		logger.System().Error(" Method not allowed: %s", r.Method)
		logger.System().Info("=== MCP REQUEST END (METHOD NOT ALLOWED) ===")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSSEConnection establishes a Server-Sent Events connection
func (s *Server) handleSSEConnection(w http.ResponseWriter, r *http.Request, mcpServer *mcp.Server) {
	logger.System().Info("=== SSE CONNECTION START ===")
	logger.System().Info("Setting up SSE connection for server: %s", mcpServer.Name)

	// Get or generate session ID
	sessionID := s.getSessionID(r)
	logger.System().Info("Session ID for SSE connection: %s", sessionID)

	// CRITICAL FIX: Register session in translator immediately
	//
	// Create an uninitialized session state so that IsInitialized() checks
	// will recognize the session exists (returning false, as expected) rather
	// than treating it as completely unknown.
	//
	// This allows the session endpoint to accept initialize requests for
	// sessions created via SSE connections.
	s.translator.RegisterSession(sessionID)
	logger.System().Info("SUCCESS: Session %s registered in translator", sessionID)

	// Create cancellable context for this connection
	// Use background context to avoid dependency on HTTP request context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Monitor HTTP request context for disconnection
	go func() {
		<-r.Context().Done()
		logger.System().Info("HTTP request context cancelled for session %s, triggering cleanup", sessionID)
		cancel()
	}()

	// Check connection limits and add to manager
	logger.System().Info("Adding connection to manager...")
	if err := s.connectionManager.AddConnection(sessionID, mcpServer.Name, ctx, cancel); err != nil {
		logger.System().Error(" Failed to add connection for session %s: %v", sessionID, err)
		logger.System().Info("=== SSE CONNECTION END (CONNECTION LIMIT) ===")
		http.Error(w, "Too many connections", http.StatusTooManyRequests)
		return
	}
	logger.System().Info("SUCCESS: Connection added to manager")

	// Set SSE headers
	logger.System().Info("Setting SSE headers...")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Session-ID", sessionID)
	logger.System().Info("SUCCESS: SSE headers set")

	// Send required "endpoint" event for Remote MCP protocol
	logger.System().Info("Sending endpoint event...")
	if _, err := fmt.Fprintf(w, "event: endpoint\n"); err != nil {
		logger.System().Error(" Failed to write SSE endpoint event: %v", err)
		logger.System().Info("=== SSE CONNECTION END (ENDPOINT EVENT FAILED) ===")
		s.connectionManager.RemoveConnection(sessionID)
		return
	}

	// Construct the session endpoint URL that Claude will use for sending messages
	logger.System().Info("INFO: Constructing session endpoint URL...")
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
	logger.System().Info("INFO: Session endpoint URL: %s", sessionEndpoint)

	endpointData := map[string]interface{}{
		"uri": sessionEndpoint,
	}

	endpointJSON, err := json.Marshal(endpointData)
	if err != nil {
		logger.System().Error(" Failed to marshal endpoint data: %v", err)
		logger.System().Info("=== SSE CONNECTION END (ENDPOINT JSON FAILED) ===")
		s.connectionManager.RemoveConnection(sessionID)
		return
	}

	logger.System().Info("INFO: Endpoint data: %s", string(endpointJSON))
	if _, err := fmt.Fprintf(w, "data: %s\n\n", string(endpointJSON)); err != nil {
		logger.System().Error(" Failed to write SSE endpoint data: %v", err)
		logger.System().Info("=== SSE CONNECTION END (ENDPOINT DATA FAILED) ===")
		s.connectionManager.RemoveConnection(sessionID)
		return
	}
	logger.System().Info("SUCCESS: Endpoint event sent successfully")

	// Flush to send the event immediately
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Clean up when connection closes
	defer func() {
		s.connectionManager.RemoveConnection(sessionID)
		s.translator.RemoveConnection(sessionID)
		s.mcpManager.CleanupSession(sessionID)
		logger.System().Info("INFO: SSE connection and session cleanup completed for server %s, session %s", mcpServer.Name, sessionID[:8])
	}()

	// Create a ticker for periodic checks and timeouts
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	logger.System().Info("INFO: Starting SSE message loop for server %s, session %s", mcpServer.Name, sessionID)

	// Add keep-alive ticker to detect client disconnection
	keepAliveTicker := time.NewTicker(30 * time.Second)
	defer keepAliveTicker.Stop()

	// Add stale connection detection
	lastActivityTime := time.Now()
	staleConnectionTimeout := 5 * time.Minute // Detect connections idle for 5+ minutes
	maxDebugMessages := 10                    // Limit debug spam
	debugMessageCount := 0

	for {
		select {
		case <-ctx.Done():
			logger.System().Info("INFO: SSE context cancelled for server %s, session %s", mcpServer.Name, sessionID)
			return
		case <-keepAliveTicker.C:
			// Send keep-alive event to detect client disconnection
			if _, err := fmt.Fprintf(w, "event: keep-alive\ndata: {\"timestamp\":%d}\n\n", time.Now().Unix()); err != nil {
				logger.System().Info("INFO: Client disconnected for session %s (server %s): %v", sessionID, mcpServer.Name, err)
				return
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			// Update activity time on successful keep-alive
			lastActivityTime = time.Now()
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

			// STALE CONNECTION DETECTION: Check if connection has been idle too long
			if time.Since(lastActivityTime) > staleConnectionTimeout {
				logger.System().Warn(" Stale SSE connection detected for server %s, session %s (idle for %v)",
					mcpServer.Name, sessionID, time.Since(lastActivityTime))
				logger.System().Info("INFO: Automatically closing stale connection to prevent resource leaks")
				return
			}

			// REDUCE DEBUG SPAM: Only log first few debug messages to prevent log flooding
			if debugMessageCount < maxDebugMessages {
				logger.System().Debug(" SSE connection active for server %s, session %s - waiting for requests", mcpServer.Name, sessionID)
				debugMessageCount++
				if debugMessageCount == maxDebugMessages {
					logger.System().Info("INFO: Debug message limit reached for session %s - silencing further debug logs", sessionID)
				}
			}

			// Just keep the connection alive - no more continuous polling
			time.Sleep(1 * time.Second)
		}
	}
}

// handleMCPMessage handles POST requests with MCP messages
func (s *Server) handleMCPMessage(w http.ResponseWriter, r *http.Request, mcpServer *mcp.Server) {
	logger.System().Info("=== MCP MESSAGE START ===")
	logger.System().Info("INFO: Processing POST message for server: %s", mcpServer.Name)

	// Read the request body
	logger.System().Info("INFO: Reading request body...")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.System().Error(" Failed to read request body: %v", err)
		logger.System().Info("=== MCP MESSAGE END (BODY READ FAILED) ===")
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	logger.System().Info("SUCCESS: Request body read (%d bytes)", len(body))
	// Smart logging - detect error content and use appropriate level
	bodyStr := string(body)
	if strings.Contains(bodyStr, "error") || strings.Contains(bodyStr, "timeout") || strings.Contains(bodyStr, "cancelled") {
		logger.System().Error("Request body contains error: %s", bodyStr)
	} else if strings.Contains(bodyStr, "Method not found") {
		logger.System().Warn("Request body: %s", bodyStr)
	} else {
		logger.System().Debug("Request body: %s", bodyStr) // Reduce verbosity to DEBUG
	}

	// Parse the JSON-RPC message to check if it's a handshake message
	logger.System().Debug("Parsing JSON-RPC message...")
	var jsonrpcMsg protocol.JSONRPCMessage
	if err := json.Unmarshal(body, &jsonrpcMsg); err != nil {
		logger.System().Error(" Invalid JSON-RPC message: %v", err)
		logger.System().Info("=== MCP MESSAGE END (JSON PARSE FAILED) ===")
		http.Error(w, fmt.Sprintf("Invalid JSON-RPC message: %v", err), http.StatusBadRequest)
		return
	}
	logger.System().Info("SUCCESS: JSON-RPC message parsed")

	// Generate or get session ID
	sessionID := s.getSessionID(r)
	logger.System().Info("INFO: Method: %s, ID: %v, SessionID: %s", jsonrpcMsg.Method, jsonrpcMsg.ID, sessionID)

	// CRITICAL FIX: Only handle handshake messages if this is NOT a session endpoint request
	//
	// Session endpoint requests (/{server}/sessions/{sessionId}) should be handled
	// entirely by handleSessionMessage to prevent duplicate ReadMessage calls.
	// The handleMCPMessage function should only handle direct SSE endpoint requests.
	//
	// Check if this request is coming to a session endpoint by looking at the URL path
	isSessionEndpointRequest := strings.Contains(r.URL.Path, "/sessions/")

	logger.System().Info("INFO: Checking request type - URL: %s, IsSessionEndpoint: %v", r.URL.Path, isSessionEndpointRequest)

	if isSessionEndpointRequest {
		logger.System().Error(" Session endpoint request incorrectly routed to handleMCPMessage")
		logger.System().Error(" This should not happen - check routing configuration")
		logger.System().Info("=== MCP MESSAGE END (ROUTING ERROR) ===")
		http.Error(w, "Internal routing error", http.StatusInternalServerError)
		return
	}

	// Handle handshake messages for direct SSE endpoint requests only
	logger.System().Info("INFO: Checking if handshake message...")
	if s.translator.IsHandshakeMessage(jsonrpcMsg.Method) {
		logger.System().Info("INFO: Processing handshake message: %s", jsonrpcMsg.Method)
		s.handleHandshakeMessage(w, r, sessionID, &jsonrpcMsg, mcpServer)
		logger.System().Info("=== MCP MESSAGE END (HANDSHAKE) ===")
		return
	}
	logger.System().Info("INFO: Not a handshake message, continuing with regular processing")

	// PROTOCOL ADAPTATION FIX: Handle Claude.ai's behavior of sending all requests to /sse
	//
	// Claude.ai continues sending requests to /sse endpoint even after initialization,
	// instead of switching to /sessions/{sessionId} as specified. To maintain compatibility
	// while following protocol standards, we forward non-handshake requests to session
	// logic when the session is initialized.
	//
	// This allows Claude.ai to work while maintaining support for compliant clients.

	// Check if connection is initialized
	if !s.translator.IsInitialized(sessionID) {
		logger.System().Error(" Session %s not initialized for non-handshake method %s", sessionID, jsonrpcMsg.Method)
		s.sendErrorResponse(w, jsonrpcMsg.ID, protocol.InvalidRequest, "Connection not initialized", false)
		return
	}

	// CRITICAL FIX: Forward non-handshake requests to session logic for initialized sessions
	//
	// When Claude.ai sends non-handshake requests (tools/list, tools/call, etc.) to the
	// /sse endpoint instead of /sessions/{sessionId}, we need to handle them synchronously
	// like the session endpoint would, rather than the old asynchronous approach.
	//
	// This ensures Claude.ai gets immediate responses for tool discovery and calls.
	logger.System().Info("INFO: Session %s is initialized, handling non-handshake request %s synchronously",
		sessionID, jsonrpcMsg.Method)

	// Track the request for potential fallback handling
	if jsonrpcMsg.Method != "" && jsonrpcMsg.ID != nil {
		s.translator.TrackRequest(sessionID, jsonrpcMsg.ID, jsonrpcMsg.Method)
		logger.System().Debug(" Tracking request ID %v, method %s for session %s", jsonrpcMsg.ID, jsonrpcMsg.Method, sessionID)
	}

	// Send request and receive response from MCP server using serialized queue
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	responseBytes, err := mcpServer.SendAndReceive(ctx, body)
	if err != nil {
		logger.System().Warn(" Failed to read response from MCP server %s for method %s: %v",
			mcpServer.Name, jsonrpcMsg.Method, err)

		// Check if we should provide a fallback response for this method
		if s.translator.ShouldProvideFallback(jsonrpcMsg.Method) {
			logger.System().Info("INFO: Providing fallback response for method %s", jsonrpcMsg.Method)
			fallbackResponse, fallbackErr := s.translator.CreateFallbackResponse(jsonrpcMsg.ID, jsonrpcMsg.Method)
			if fallbackErr == nil {
				responseBytes = fallbackResponse
			} else {
				logger.System().Error(" Failed to create fallback response: %v", fallbackErr)
				http.Error(w, "Failed to receive response from MCP server", http.StatusInternalServerError)
				return
			}
		} else {
			// For non-fallback methods, create a proper error response
			logger.System().Info("INFO: Creating error response for unsupported method %s", jsonrpcMsg.Method)
			errorResponse, errorErr := s.translator.CreateErrorResponse(jsonrpcMsg.ID,
				protocol.MethodNotFound, fmt.Sprintf("Method %s not supported", jsonrpcMsg.Method), false)
			if errorErr == nil {
				responseBytes = errorResponse
			} else {
				logger.System().Error(" Failed to create error response: %v", errorErr)
				http.Error(w, "Failed to receive response from MCP server", http.StatusInternalServerError)
				return
			}
		}
	}

	// Return response directly to Claude.ai (synchronous like session endpoint)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Mcp-Session-Id", sessionID)
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(responseBytes); err != nil {
		logger.System().Error(" Failed to write synchronous response: %v", err)
	} else {
		logger.System().Info("INFO: Successfully returned synchronous response for %s to session %s via /sse endpoint",
			jsonrpcMsg.Method, sessionID)
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
	logger.System().Info("%s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

	// Log headers for debugging Remote MCP protocol
	for name, values := range r.Header {
		if strings.HasPrefix(strings.ToLower(name), "x-") ||
			strings.Contains(strings.ToLower(name), "mcp") {
			logger.System().Info("Header %s: %v", name, values)
		}
	}
}

// getSessionID generates or retrieves a session ID for the request
func (s *Server) getSessionID(r *http.Request) string {
	// Try to get session ID from Remote MCP standard header
	if sessionID := r.Header.Get("Mcp-Session-Id"); sessionID != "" {
		logger.System().Debug(" Using existing session ID from Mcp-Session-Id header: %s", sessionID)
		return sessionID
	}

	// Try to get session ID from legacy header for backward compatibility
	if sessionID := r.Header.Get("X-Session-ID"); sessionID != "" {
		logger.System().Debug(" Using existing session ID from X-Session-ID header: %s", sessionID)
		return sessionID
	}

	// Generate a new session ID
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		logger.System().Error(" Failed to generate random session ID: %v", err)
		// Fallback to a simple timestamp-based ID
		return fmt.Sprintf("session-%d", time.Now().UnixNano())
	}

	sessionID := hex.EncodeToString(bytes)
	logger.System().Debug("Generated session ID: %s", sessionID)
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
		logger.System().Error(" MCP server '%s' is not running for initialize", mcpServer.Name)
		s.sendErrorResponse(w, msg.ID, protocol.InvalidRequest, fmt.Sprintf("MCP server '%s' is not running", mcpServer.Name), false)
		return
	}

	// Forward the initialize request to the actual MCP server
	initRequestBytes, err := json.Marshal(msg)
	if err != nil {
		logger.System().Error(" Failed to marshal initialize request: %v", err)
		s.sendErrorResponse(w, msg.ID, protocol.InternalError, "Failed to process initialize request", false)
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
	// The serialized request queue prevents stdio deadlocks and response mismatching that
	// occur when multiple concurrent requests try to access the same MCP server simultaneously.
	logger.System().Info("INFO: Waiting for initialize response from MCP server %s...", mcpServer.Name)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Send initialize request and receive response using serialized queue
	responseBytes, err := mcpServer.SendAndReceive(ctx, initRequestBytes)
	if err != nil {
		logger.System().Error(" Failed to send/receive initialize request to MCP server %s: %v", mcpServer.Name, err)

		// CRITICAL FIX: Attempt server restart on initialize timeout
		if strings.Contains(err.Error(), "context deadline exceeded") {
			logger.System().Warn(" MCP server %s appears hung, attempting restart...", mcpServer.Name)
			if restartErr := s.mcpManager.RestartServer(mcpServer.Name); restartErr != nil {
				logger.System().Error(" Failed to restart MCP server %s: %v", mcpServer.Name, restartErr)
			} else {
				logger.System().Info("INFO: Successfully restarted MCP server %s", mcpServer.Name)
				// Retry initialize with new server instance
				retryCtx, retryCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer retryCancel()
				if retryBytes, retryErr := mcpServer.SendAndReceive(retryCtx, initRequestBytes); retryErr == nil {
					logger.System().Info("INFO: Initialize retry succeeded for server %s after restart", mcpServer.Name)
					responseBytes = retryBytes
					err = nil
				} else {
					logger.System().Error(" Initialize retry failed for server %s: %v", mcpServer.Name, retryErr)
				}
			}
		}

		if err != nil {
			s.sendErrorResponse(w, msg.ID, protocol.InternalError, "Failed to communicate with MCP server", false)
			return
		}
	}

	// Parse the MCP server's initialize response
	var mcpResponse protocol.JSONRPCMessage
	if err := json.Unmarshal(responseBytes, &mcpResponse); err != nil {
		logger.System().Error(" Failed to parse initialize response from MCP server %s: %v", mcpServer.Name, err)
		s.sendErrorResponse(w, msg.ID, protocol.InternalError, "Invalid response from MCP server", false)
		return
	}

	// Store connection state in translator
	if mcpResponse.Result != nil {
		_, err := s.translator.HandleInitialize(sessionID, params)
		if err != nil {
			logger.System().Error(" Failed to store connection state: %v", err)
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
				logger.System().Error(" Failed to mark session as initialized: %v", err)
			} else {
				logger.System().Info("INFO: Session %s marked as initialized for server %s", sessionID, mcpServer.Name)
			}
		}
	}

	// Return the MCP server's response directly to Claude
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Session-ID", sessionID)
	w.Header().Set("Mcp-Session-Id", sessionID)
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(responseBytes); err != nil {
		logger.System().Error(" Failed to write initialize response: %v", err)
	} else {
		logger.System().Info("INFO: Forwarded initialize response from MCP server %s for session %s", mcpServer.Name, sessionID)
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

	logger.System().Info("Connection initialized for session %s", sessionID)
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
			logger.System().Debug(" Using server name '%s' from URL path for session", serverName)
		} else {
			logger.System().Error(" No server name found in context or URL path for host: %s, path: %s", r.Host, r.URL.Path)
			http.Error(w, "Invalid request format. Expected: {server}.mcp.{domain}/sessions/{id} or /{server}/sessions/{id}", http.StatusBadRequest)
			return
		}
	}

	vars := mux.Vars(r)
	sessionID := vars["sessionId"]

	logger.System().Debug("=== SESSION MESSAGE START ===")
	logger.System().Info("Handling session message for server: %s, session: %s", serverName, sessionID)
	logger.System().Debug("Method: %s, URL: %s", r.Method, r.URL.String())
	logger.System().Debug("Remote Address: %s", r.RemoteAddr)
	logger.System().Debug("User-Agent: %s", r.Header.Get("User-Agent"))
	logger.System().Debug("Content-Type: %s", r.Header.Get("Content-Type"))

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
	logger.System().Debug("Reading session message body...")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.System().Error(" Failed to read session message body: %v", err)
		logger.System().Info("=== SESSION MESSAGE END (BODY READ FAILED) ===")
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	logger.System().Info("SUCCESS: Session message body read (%d bytes)", len(body))
	logger.System().Debug("Session message body: %s", string(body))

	// Parse the JSON-RPC message
	logger.System().Debug("Parsing session message JSON-RPC...")
	var jsonrpcMsg protocol.JSONRPCMessage
	if err := json.Unmarshal(body, &jsonrpcMsg); err != nil {
		logger.System().Error(" Invalid session message JSON-RPC: %v", err)
		logger.System().Info("=== SESSION MESSAGE END (JSON PARSE FAILED) ===")
		http.Error(w, fmt.Sprintf("Invalid JSON-RPC message: %v", err), http.StatusBadRequest)
		return
	}
	logger.System().Info("SUCCESS: Session message JSON-RPC parsed")
	logger.System().Debug("Session message method: %s, ID: %v, SessionID: %s", jsonrpcMsg.Method, jsonrpcMsg.ID, sessionID)

	// CRITICAL FIX: Allow handshake messages on uninitialized sessions
	//
	// For Remote MCP protocol compliance, the session endpoint must accept
	// initialize requests even when the session is not yet initialized.
	// This breaks the deadlock where Claude.ai cannot initialize because
	// session endpoints reject uninitialized requests.

	isHandshake := s.translator.IsHandshakeMessage(jsonrpcMsg.Method)
	isInitialized := s.translator.IsInitialized(sessionID)

	logger.System().Debug(" Session %s - Method: %s, IsHandshake: %v, IsInitialized: %v",
		sessionID, jsonrpcMsg.Method, isHandshake, isInitialized)

	if !isHandshake && !isInitialized {
		logger.System().Error(" Session %s not initialized for non-handshake method %s", sessionID, jsonrpcMsg.Method)
		http.Error(w, "Session not initialized", http.StatusBadRequest)
		return
	}

	// Track the request for potential fallback handling
	if jsonrpcMsg.Method != "" && jsonrpcMsg.ID != nil {
		s.translator.TrackRequest(sessionID, jsonrpcMsg.ID, jsonrpcMsg.Method)
		logger.System().Debug(" Tracking session request ID %v, method %s for session %s", jsonrpcMsg.ID, jsonrpcMsg.Method, sessionID)
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

	logger.System().Info("INFO: Handling session request %s synchronously", jsonrpcMsg.Method)

	// Send request and receive response from MCP server using serialized queue
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	responseBytes, err := mcpServer.SendAndReceive(ctx, body)
	if err != nil {
		logger.System().Error(" Failed to send/receive message to MCP server %s: %v", serverName, err)
		http.Error(w, "Failed to communicate with MCP server", http.StatusInternalServerError)
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
					logger.System().Error(" Failed to mark session as initialized: %v", err)
				} else {
					logger.System().Info("INFO: Session %s marked as initialized for server %s", sessionID, serverName)
				}
			}
		}
	}

	// Return response directly to Claude.ai
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Mcp-Session-Id", sessionID)
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(responseBytes); err != nil {
		logger.System().Error(" Failed to write session response: %v", err)
	} else {
		logger.System().Info("INFO: Successfully returned synchronous response for %s to session %s", jsonrpcMsg.Method, sessionID)
	}
}

// sendErrorResponse sends a JSON-RPC error response
func (s *Server) sendErrorResponse(w http.ResponseWriter, id interface{}, code int, message string, isRemoteMCP bool) {
	logger.System().Error(" Sending error response - Code: %d, Message: %s", code, message)

	errorResponse, err := s.translator.CreateErrorResponse(id, code, message, isRemoteMCP)
	if err != nil {
		logger.System().Error(" Failed to create error response: %v", err)
		http.Error(w, "Failed to create error response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(errorResponse); err != nil {
		logger.System().Error(" Failed to write error response: %v", err)
	} else {
		logger.System().Debug(" Error response sent successfully")
	}
}

// validateAuthentication validates the authentication for the request
func (s *Server) validateAuthentication(r *http.Request) bool {
	// Check for Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		logger.System().Error(" No authorization header found, authentication required")
		return false
	}

	// Parse Bearer token
	if !strings.HasPrefix(authHeader, "Bearer ") {
		logger.System().Error(" Invalid authorization header format, expected Bearer token")
		return false
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		logger.System().Error(" Empty bearer token")
		return false
	}

	// Simple token validation - accept any non-empty token for Claude.ai compatibility
	// For Claude.ai Remote MCP, any Bearer token should work
	logger.System().Debug("Authentication successful with token: %s...", func() string {
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

	logger.System().Info("Origin not allowed: %s", origin)
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

	logger.System().Info("OAuth client registered - ID: %s", clientID)

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

	logger.System().Info("OAuth authorization request - Client: %s, Redirect: %s", clientID, redirectURI)

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

	logger.System().Info("OAuth token issued - Client: %s, Token: %s...", clientID, accessToken[:10])

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

// handleServerHealth returns health status for all MCP servers
func (s *Server) handleServerHealth(w http.ResponseWriter, r *http.Request) {
	logger.System().Info("Handling server health check request")

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if s.healthChecker == nil {
		http.Error(w, "Health checker not available", http.StatusServiceUnavailable)
		return
	}

	healthStatus := s.healthChecker.GetHealthStatus()

	response := map[string]interface{}{
		"timestamp": time.Now(),
		"servers":   healthStatus,
		"summary": map[string]int{
			"total":     len(healthStatus),
			"healthy":   0,
			"unhealthy": 0,
			"unknown":   0,
		},
	}

	// Calculate summary
	for _, health := range healthStatus {
		switch health.Status {
		case "healthy":
			response["summary"].(map[string]int)["healthy"]++
		case "unhealthy":
			response["summary"].(map[string]int)["unhealthy"]++
		default:
			response["summary"].(map[string]int)["unknown"]++
		}
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.System().Error("Failed to encode server health response: %v", err)
	}
}

// handleResourceMetrics returns resource usage metrics for MCP processes
func (s *Server) handleResourceMetrics(w http.ResponseWriter, r *http.Request) {
	logger.System().Info("Handling resource metrics request")

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if s.resourceMonitor == nil {
		http.Error(w, "Resource monitor not available", http.StatusServiceUnavailable)
		return
	}

	metrics, err := s.resourceMonitor.GetCurrentMetrics()
	if err != nil {
		logger.System().Error("Failed to get resource metrics: %v", err)
		http.Error(w, "Failed to get resource metrics", http.StatusInternalServerError)
		return
	}

	// Calculate totals
	totalMemoryMB := 0.0
	totalCPU := 0.0

	for _, metric := range metrics {
		totalMemoryMB += metric.MemoryMB
		totalCPU += metric.CPUPercent
	}

	response := map[string]interface{}{
		"timestamp": time.Now(),
		"processes": metrics,
		"summary": map[string]interface{}{
			"processCount":  len(metrics),
			"totalMemoryMB": totalMemoryMB,
			"totalCPU":      totalCPU,
			"averageMemoryMB": func() float64 {
				if len(metrics) > 0 {
					return totalMemoryMB / float64(len(metrics))
				}
				return 0
			}(),
			"averageCPU": func() float64 {
				if len(metrics) > 0 {
					return totalCPU / float64(len(metrics))
				}
				return 0
			}(),
		},
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.System().Error("Failed to encode resource metrics response: %v", err)
	}
}
