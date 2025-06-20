package proxy

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"remote-mcp-proxy/mcp"
	"remote-mcp-proxy/protocol"
)

// Server represents the HTTP proxy server
type Server struct {
	mcpManager *mcp.Manager
	translator *protocol.Translator
}

// NewServer creates a new proxy server
func NewServer(mcpManager *mcp.Manager) *Server {
	return &Server{
		mcpManager: mcpManager,
		translator: protocol.NewTranslator(),
	}
}

// Router returns the HTTP router with all routes configured
func (s *Server) Router() http.Handler {
	r := mux.NewRouter()

	// Health check endpoint
	r.HandleFunc("/health", s.handleHealth).Methods("GET")

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
	w.Write([]byte(`{"status":"healthy"}`))
}

// handleMCPRequest handles Remote MCP requests and forwards them to local MCP servers
func (s *Server) handleMCPRequest(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	serverName := vars["server"]

	log.Printf("Handling MCP request for server: %s", serverName)

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
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\n")
	fmt.Fprintf(w, "data: {\"type\":\"connected\",\"server\":\"%s\"}\n\n", mcpServer.Name)

	// Flush to send the event immediately
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	// Keep connection alive and listen for MCP messages
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			log.Printf("SSE connection closed for server: %s", mcpServer.Name)
			return
		default:
			// Read message from MCP server (non-blocking)
			message, err := mcpServer.ReadMessage()
			if err != nil {
				if err.Error() != "EOF" {
					log.Printf("Error reading from MCP server %s: %v", mcpServer.Name, err)
				}
				continue
			}

			// Translate and send message
			remoteMCPMessage, err := s.translator.MCPToRemote(message)
			if err != nil {
				log.Printf("Error translating MCP message: %v", err)
				continue
			}

			fmt.Fprintf(w, "event: message\n")
			fmt.Fprintf(w, "data: %s\n\n", string(remoteMCPMessage))

			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}
}

// handleMCPMessage handles POST requests with MCP messages
func (s *Server) handleMCPMessage(w http.ResponseWriter, r *http.Request, mcpServer *mcp.Server) {
	// Read the request body
	body := make([]byte, r.ContentLength)
	_, err := r.Body.Read(body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
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

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"sent"}`))
}

// corsMiddleware adds CORS headers to all responses
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

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