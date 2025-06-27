package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"remote-mcp-proxy/config"
	"remote-mcp-proxy/logger"
)

// RequestResponse represents a paired request/response for serialization
type RequestResponse struct {
	Request    []byte
	ResponseCh chan RequestResult
	Ctx        context.Context
}

// RequestResult contains the response and any error
type RequestResult struct {
	Response []byte
	Error    error
}

// OperationInfo tracks information about active MCP operations
type OperationInfo struct {
	RequestID string    // Unique identifier for the request
	Method    string    // MCP method being executed (e.g., "tools/call")
	StartTime time.Time // When the operation started
	SessionID string    // Session that initiated the operation
	ToolName  string    // Name of tool being called (for tools/call operations)
}

// Server represents a running MCP server process
type Server struct {
	Name    string
	Config  config.MCPServer
	Process *exec.Cmd
	Stdin   io.WriteCloser
	Stdout  io.ReadCloser
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.RWMutex
	logger  *logger.Logger

	// CRITICAL FIX: Dedicated mutex for stdout reading to prevent stdio deadlocks
	//
	// This mutex serializes access to the MCP server's stdout stream, preventing
	// race conditions when multiple concurrent HTTP requests try to read responses
	// from the same MCP server simultaneously.
	//
	// Without this mutex, bufio.Scanner.Scan() calls can deadlock or interfere
	// with each other, causing "context deadline exceeded" errors during
	// initialization and other requests.
	//
	// DO NOT REMOVE - this is essential for concurrent request handling
	readMu sync.Mutex

	// CONCURRENCY FIX: Request serialization to prevent response mismatching
	//
	// This channel-based queue ensures that requests to the same MCP server
	// are processed one at a time, preventing multiple sessions from interfering
	// with each other's responses when accessing the same MCP server.
	//
	// Each request gets a dedicated response channel to ensure proper correlation.
	requestQueue chan RequestResponse
	queueStarted bool

	// OPERATION TRACKING: Track active operations to prevent premature server termination
	//
	// These fields enable operation-aware cleanup that respects long-running operations
	// like Sequential Thinking's multi-thought processes. Servers with active operations
	// will not be terminated during session cleanup.
	activeOperations    map[string]*OperationInfo // requestID -> operation info
	operationsMu        sync.RWMutex              // Protects activeOperations map
	lastOperationTime   time.Time                 // Last time an operation started
	operationTimeoutSec int                       // Server-specific operation timeout
}

// Manager manages multiple MCP server processes
type Manager struct {
	servers        map[string]*Server            // Global servers (legacy mode)
	sessionServers map[string]map[string]*Server // sessionID -> serverName -> Server
	configs        map[string]config.MCPServer   // Server configurations
	mu             sync.RWMutex
}

// NewManager creates a new MCP manager
func NewManager(configs map[string]config.MCPServer) *Manager {
	m := &Manager{
		servers:        make(map[string]*Server),
		sessionServers: make(map[string]map[string]*Server),
		configs:        make(map[string]config.MCPServer),
	}

	// Store configurations for later use
	for name, cfg := range configs {
		m.configs[name] = cfg
	}

	// Initialize global servers from configs (legacy mode)
	for name, cfg := range configs {
		// Get MCP logger for this server
		mcpLogger, err := logger.MCP(name)
		if err != nil {
			// Fallback to system logger if MCP logger fails
			logger.System().Error("Failed to create MCP logger for %s: %v", name, err)
			mcpLogger = logger.System()
		}

		// Set reasonable default operation timeout for all MCP servers
		// Since we have real-time operation monitoring and intelligent cleanup
		// that protects active operations, we only need a timeout for truly stuck operations
		operationTimeout := 300 // 5 minutes default - reasonable for any MCP operation

		m.servers[name] = &Server{
			Name:                name,
			Config:              cfg,
			requestQueue:        make(chan RequestResponse, 100), // Buffer for concurrent requests
			queueStarted:        false,
			logger:              mcpLogger,
			activeOperations:    make(map[string]*OperationInfo),
			lastOperationTime:   time.Time{}, // Zero time initially
			operationTimeoutSec: operationTimeout,
		}
	}

	return m
}

// StartAll starts all configured MCP servers
func (m *Manager) StartAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, server := range m.servers {
		if err := m.startServer(name, server.Config); err != nil {
			return fmt.Errorf("failed to start server %s: %w", name, err)
		}
	}

	return nil
}

// GetServer returns a server by name (legacy global mode)
func (m *Manager) GetServer(name string) (*Server, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	server, exists := m.servers[name]
	return server, exists
}

// GetServerForSession returns a session-specific server, creating it if needed
func (m *Manager) GetServerForSession(sessionID, serverName string) (*Server, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if session exists
	sessionMap, sessionExists := m.sessionServers[sessionID]
	if !sessionExists {
		// Create new session map
		sessionMap = make(map[string]*Server)
		m.sessionServers[sessionID] = sessionMap
	}

	// Check if server exists for this session
	server, serverExists := sessionMap[serverName]
	if serverExists {
		return server, true
	}

	// Check if we have config for this server
	cfg, configExists := m.configs[serverName]
	if !configExists {
		logger.System().Error("No configuration found for server %s", serverName)
		return nil, false
	}

	// Create session-aware configuration
	sessionCfg := m.createSessionConfig(sessionID, serverName, cfg)

	// Create new server instance for this session
	mcpLogger, err := logger.MCP(fmt.Sprintf("%s-%s", serverName, sessionID[:8]))
	if err != nil {
		logger.System().Error("Failed to create MCP logger for %s-%s: %v", serverName, sessionID[:8], err)
		mcpLogger = logger.System()
	}

	server = &Server{
		Name:         fmt.Sprintf("%s-%s", serverName, sessionID[:8]),
		Config:       sessionCfg,
		requestQueue: make(chan RequestResponse, 100),
		queueStarted: false,
		logger:       mcpLogger,
	}

	// Start the server
	if err := m.startServerForSession(sessionID, serverName, server); err != nil {
		logger.System().Error("Failed to start server %s for session %s: %v", serverName, sessionID, err)
		return nil, false
	}

	// Store the server
	sessionMap[serverName] = server

	return server, true
}

// createSessionConfig creates a session-aware configuration with template substitution
func (m *Manager) createSessionConfig(sessionID, serverName string, baseCfg config.MCPServer) config.MCPServer {
	// Create a copy of the base config
	sessionCfg := config.MCPServer{
		Command: baseCfg.Command,
		Args:    make([]string, len(baseCfg.Args)),
		Env:     make(map[string]string),
	}

	// Copy and substitute args with template variables
	for i, arg := range baseCfg.Args {
		arg = strings.ReplaceAll(arg, "{SESSION_ID}", sessionID)
		arg = strings.ReplaceAll(arg, "{SERVER_NAME}", serverName)
		sessionCfg.Args[i] = arg
	}

	// Copy and substitute environment variables
	for key, value := range baseCfg.Env {
		// Replace template variables
		value = strings.ReplaceAll(value, "{SESSION_ID}", sessionID)
		value = strings.ReplaceAll(value, "{SERVER_NAME}", serverName)
		sessionCfg.Env[key] = value
	}

	return sessionCfg
}

// startServerForSession starts a server for a specific session with session-aware directory setup
func (m *Manager) startServerForSession(sessionID, serverName string, server *Server) error {
	// Create session directory
	sessionDir := fmt.Sprintf("/app/sessions/%s", sessionID)
	if err := m.ensureSessionDirectory(sessionDir); err != nil {
		return fmt.Errorf("failed to create session directory: %w", err)
	}

	logger.System().Info("Starting MCP server %s for session %s", serverName, sessionID[:8])

	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, server.Config.Command, server.Config.Args...)

	// Set environment variables
	cmd.Env = os.Environ()
	for key, value := range server.Config.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
	}

	// Set working directory to session directory
	cmd.Dir = sessionDir

	// Set up pipes for communication
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		logger.System().Error("Failed to create stdin pipe for server %s-%s: %v", serverName, sessionID[:8], err)
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		if stdin != nil {
			stdin.Close()
		}
		logger.System().Error("Failed to create stdout pipe for server %s-%s: %v", serverName, sessionID[:8], err)
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		cancel()
		if stdin != nil {
			stdin.Close()
		}
		if stdout != nil {
			stdout.Close()
		}
		logger.System().Error("Failed to start process for server %s-%s: %v", serverName, sessionID[:8], err)
		return fmt.Errorf("failed to start process: %w", err)
	}

	// Update the server with process information
	server.Process = cmd
	server.Stdin = stdin
	server.Stdout = stdout
	server.ctx = ctx
	server.cancel = cancel

	// Start the request processor if not already started
	if !server.queueStarted {
		go server.processRequests()
		server.queueStarted = true
	}

	// Start monitoring the process
	go server.monitor()

	logger.System().Info("Successfully started MCP server %s-%s (PID: %d)", serverName, sessionID[:8], cmd.Process.Pid)
	return nil
}

// ensureSessionDirectory creates the session directory and any necessary subdirectories
func (m *Manager) ensureSessionDirectory(sessionDir string) error {
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return err
	}

	// Create common subdirectories that MCP servers might need
	subdirs := []string{"data", "cache", "temp"}
	for _, subdir := range subdirs {
		fullPath := fmt.Sprintf("%s/%s", sessionDir, subdir)
		if err := os.MkdirAll(fullPath, 0755); err != nil {
			logger.System().Warn("Failed to create subdirectory %s: %v", fullPath, err)
		}
	}

	return nil
}

// CleanupSession stops all servers for a session and cleans up resources
func (m *Manager) CleanupSession(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessionMap, exists := m.sessionServers[sessionID]
	if !exists {
		logger.System().Debug("No servers found for session %s during cleanup", sessionID[:8])
		return
	}

	logger.System().Info("Cleaning up session %s with %d servers", sessionID[:8], len(sessionMap))

	// Stop all servers for this session
	for serverName, server := range sessionMap {
		logger.System().Info("Stopping server %s for session %s", serverName, sessionID[:8])
		server.Stop()
	}

	// Remove session from tracking
	delete(m.sessionServers, sessionID)

	// Clean up session directory (optional - could be kept for persistence)
	sessionDir := fmt.Sprintf("/app/sessions/%s", sessionID)
	if err := os.RemoveAll(sessionDir); err != nil {
		logger.System().Warn("Failed to clean up session directory %s: %v", sessionDir, err)
	} else {
		logger.System().Info("Cleaned up session directory for session %s", sessionID[:8])
	}
}

// GetSessionServers returns information about all servers for a specific session
func (m *Manager) GetSessionServers(sessionID string) []ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessionMap, exists := m.sessionServers[sessionID]
	if !exists {
		return []ServerStatus{}
	}

	var statuses []ServerStatus
	for serverName, server := range sessionMap {
		status := ServerStatus{
			Name:    fmt.Sprintf("%s-%s", serverName, sessionID[:8]),
			Command: server.Config.Command,
			Args:    server.Config.Args,
		}

		server.mu.RLock()
		if server.Process != nil && server.Process.Process != nil {
			status.Running = true
			status.PID = server.Process.Process.Pid
		} else {
			status.Running = false
		}
		server.mu.RUnlock()

		statuses = append(statuses, status)
	}

	return statuses
}

// GetSessionServerMap returns the actual server objects for a session (for operation tracking)
func (m *Manager) GetSessionServerMap(sessionID string) map[string]*Server {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessionMap, exists := m.sessionServers[sessionID]
	if !exists {
		return make(map[string]*Server)
	}

	// Return a copy of the map to avoid concurrent access issues
	result := make(map[string]*Server)
	for name, server := range sessionMap {
		result[name] = server
	}
	return result
}

// ServerStatus represents the status of an MCP server
type ServerStatus struct {
	Name    string   `json:"name"`
	Running bool     `json:"running"`
	PID     int      `json:"pid,omitempty"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// GetAllServers returns status information for all configured servers
func (m *Manager) GetAllServers() []ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var statuses []ServerStatus
	for name, server := range m.servers {
		status := ServerStatus{
			Name:    name,
			Command: server.Config.Command,
			Args:    server.Config.Args,
		}

		server.mu.RLock()
		if server.Process != nil && server.Process.Process != nil {
			status.Running = true
			status.PID = server.Process.Process.Pid
		} else {
			status.Running = false
		}
		server.mu.RUnlock()

		statuses = append(statuses, status)
	}

	return statuses
}

// IsRunning checks if the server is currently running
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Process != nil && s.Process.Process != nil
}

// StopAll stops all running MCP servers
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, server := range m.servers {
		logger.System().Info("Stopping MCP server: %s", name)
		server.Stop()
	}
}

// startServer starts a single MCP server
// NOTE: This method must be called with m.mu locked
func (m *Manager) startServer(name string, cfg config.MCPServer) error {
	logger.System().Info("Starting MCP server: %s", name)

	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)

	// Set environment variables
	cmd.Env = os.Environ()
	for key, value := range cfg.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
	}

	// Set up pipes for communication
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		logger.System().Error("Failed to create stdin pipe for server %s: %v", name, err)
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		if stdin != nil {
			stdin.Close()
		}
		logger.System().Error("Failed to create stdout pipe for server %s: %v", name, err)
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		cancel()
		if stdin != nil {
			stdin.Close()
		}
		if stdout != nil {
			stdout.Close()
		}
		logger.System().Error("Failed to start process for server %s: %v", name, err)
		return fmt.Errorf("failed to start process: %w", err)
	}

	// Update the existing server with process information (mutex is already held by caller)
	server := m.servers[name]
	server.Process = cmd
	server.Stdin = stdin
	server.Stdout = stdout
	server.ctx = ctx
	server.cancel = cancel

	// Start the request processor if not already started
	if !server.queueStarted {
		go server.processRequests()
		server.queueStarted = true
	}

	// Start monitoring the process
	go server.monitor()

	logger.System().Info("Successfully started MCP server %s (PID: %d)", name, cmd.Process.Pid)
	return nil
}

// Stop gracefully stops the MCP server
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Info("Stopping MCP server: %s", s.Name)

	if s.Process == nil {
		s.logger.Warn("Server %s already stopped or not started", s.Name)
		// Still clean up pipes even if no process
		if s.Stdin != nil {
			if err := s.Stdin.Close(); err != nil {
				s.logger.Error("Failed to close stdin for server %s: %v", s.Name, err)
			}
			s.Stdin = nil
		}
		if s.Stdout != nil {
			if err := s.Stdout.Close(); err != nil {
				s.logger.Error("Failed to close stdout for server %s: %v", s.Name, err)
			}
			s.Stdout = nil
		}
		return
	}

	// Cancel context to signal shutdown
	if s.cancel != nil {
		s.cancel()
	}

	// Close pipes to release resources and signal the process to exit
	if s.Stdin != nil {
		if err := s.Stdin.Close(); err != nil {
			s.logger.Error("Failed to close stdin for server %s: %v", s.Name, err)
		} else {
			s.logger.Debug("Closed stdin for server %s", s.Name)
		}
		s.Stdin = nil
	}

	if s.Stdout != nil {
		if err := s.Stdout.Close(); err != nil {
			s.logger.Error("Failed to close stdout for server %s: %v", s.Name, err)
		} else {
			s.logger.Debug("Closed stdout for server %s", s.Name)
		}
		s.Stdout = nil
	}

	// Wait for process to exit gracefully
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("Panic in process wait for server %s: %v", s.Name, r)
				done <- fmt.Errorf("panic in process wait: %v", r)
			}
		}()

		err := s.Process.Wait()
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			s.logger.Warn("MCP server %s exited with error: %v", s.Name, err)
		} else {
			s.logger.Info("MCP server %s exited gracefully", s.Name)
		}
	case <-time.After(10 * time.Second):
		// Force kill if graceful shutdown takes too long
		s.logger.Warn("Force killing MCP server %s after timeout", s.Name)
		if s.Process.Process != nil {
			if err := s.Process.Process.Signal(syscall.SIGKILL); err != nil {
				s.logger.Error("Failed to kill process for server %s: %v", s.Name, err)
			} else {
				s.logger.Info("Sent SIGKILL to server %s", s.Name)
			}
		}

		// Wait a bit more for the forced kill to take effect
		select {
		case err := <-done:
			if err != nil {
				s.logger.Info("Server %s terminated after SIGKILL with error: %v", s.Name, err)
			} else {
				s.logger.Info("Server %s terminated after SIGKILL", s.Name)
			}
		case <-time.After(2 * time.Second):
			s.logger.Error("Server %s did not respond to SIGKILL", s.Name)
		}
	}

	s.Process = nil
	s.logger.Info("Server %s stop completed", s.Name)
}

// processRequests handles serialized request processing for the MCP server
func (s *Server) processRequests() {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("Panic in processRequests goroutine for server %s: %v", s.Name, r)
		}
		s.logger.Info("Request processor goroutine exiting for server %s", s.Name)
	}()

	s.logger.Info("Starting request processor for server %s", s.Name)

	for {
		select {
		case req := <-s.requestQueue:
			// Process the request synchronously
			s.processRequest(req)
		case <-s.ctx.Done():
			s.logger.Info("Request processor context cancelled for server %s", s.Name)
			return
		}
	}
}

// processRequest handles a single request/response cycle
func (s *Server) processRequest(req RequestResponse) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("Panic in processRequest for server %s: %v", s.Name, r)
			req.ResponseCh <- RequestResult{nil, fmt.Errorf("panic in request processing: %v", r)}
		}
		close(req.ResponseCh)
	}()

	// Send the request
	if err := s.sendMessageDirect(req.Request); err != nil {
		req.ResponseCh <- RequestResult{nil, err}
		return
	}

	// Read the response
	response, err := s.readMessageDirect(req.Ctx)
	req.ResponseCh <- RequestResult{response, err}
}

// sendMessageDirect sends a message directly (internal use by request processor)
func (s *Server) sendMessageDirect(message []byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Removed redundant server name logging - server context already available in MCP logs

	if s.Stdin == nil {
		s.logger.Error("Cannot send message to server %s: server not running", s.Name)
		return fmt.Errorf("server not running")
	}

	_, err := s.Stdin.Write(append(message, '\n'))
	if err != nil {
		s.logger.Error("Failed to send message to server %s: %v", s.Name, err)
		s.logger.Debug("<<< %s FAILED", s.Name)
		return err
	}

	s.logger.Debug("<<< %s OK", s.Name)
	return nil
}

// SendAndReceive sends a request and waits for the response using the serialized queue
func (s *Server) SendAndReceive(ctx context.Context, message []byte) ([]byte, error) {
	// OPERATION TRACKING: Parse request to extract operation information
	operationInfo := s.parseOperationInfo(message, ctx)
	if operationInfo != nil {
		s.startOperation(operationInfo)
		defer s.endOperation(operationInfo.RequestID)
	}

	// Create response channel
	responseCh := make(chan RequestResult, 1)

	// Create request
	req := RequestResponse{
		Request:    message,
		ResponseCh: responseCh,
		Ctx:        ctx,
	}

	// Send to queue
	select {
	case s.requestQueue <- req:
		// Request queued successfully
	case <-ctx.Done():
		s.logger.Error("Context cancelled before queuing request for server %s", s.Name)
		return nil, ctx.Err()
	}

	// Wait for response
	select {
	case result := <-responseCh:
		if result.Error != nil {
			s.logger.Error("Failed to process request for server %s: %v", s.Name, result.Error)
			// Removed redundant server name logging - error details already logged
			return nil, result.Error
		}
		// Removed redundant server name logging - server context already available in MCP logs
		return result.Response, nil
	case <-ctx.Done():
		s.logger.Error("Context cancelled while waiting for response from server %s", s.Name)
		return nil, ctx.Err()
	}
}

// parseOperationInfo extracts operation information from a JSON-RPC request
func (s *Server) parseOperationInfo(message []byte, ctx context.Context) *OperationInfo {
	var jsonrpcMsg map[string]interface{}
	if err := json.Unmarshal(message, &jsonrpcMsg); err != nil {
		return nil // Not a valid JSON-RPC message, skip tracking
	}

	method, ok := jsonrpcMsg["method"].(string)
	if !ok {
		return nil // No method field, skip tracking
	}

	requestID, ok := jsonrpcMsg["id"]
	if !ok {
		return nil // No request ID, skip tracking
	}

	// Generate unique operation ID
	opID := fmt.Sprintf("%s-%v-%d", s.Name, requestID, time.Now().UnixNano())

	// Extract session ID from context if available
	sessionID := ""
	if session := ctx.Value("sessionID"); session != nil {
		if sessionStr, ok := session.(string); ok {
			sessionID = sessionStr
		}
	}

	// Extract tool name for tools/call operations
	toolName := ""
	if method == "tools/call" {
		if params, ok := jsonrpcMsg["params"].(map[string]interface{}); ok {
			if name, ok := params["name"].(string); ok {
				toolName = name
			}
		}
	}

	return &OperationInfo{
		RequestID: opID,
		Method:    method,
		StartTime: time.Now(),
		SessionID: sessionID,
		ToolName:  toolName,
	}
}

// startOperation registers the start of an operation
func (s *Server) startOperation(info *OperationInfo) {
	s.operationsMu.Lock()
	defer s.operationsMu.Unlock()

	s.activeOperations[info.RequestID] = info
	s.lastOperationTime = info.StartTime

	s.logger.Info("OPERATION START: %s %s (tool: %s) on server %s",
		info.Method, info.RequestID[:8], info.ToolName, s.Name)
}

// endOperation marks an operation as completed
func (s *Server) endOperation(requestID string) {
	s.operationsMu.Lock()
	defer s.operationsMu.Unlock()

	if info, exists := s.activeOperations[requestID]; exists {
		duration := time.Since(info.StartTime)
		delete(s.activeOperations, requestID)

		s.logger.Info("OPERATION END: %s %s completed in %v on server %s",
			info.Method, requestID[:8], duration, s.Name)
	}
}

// HasActiveOperations returns true if the server has any active operations
func (s *Server) HasActiveOperations() bool {
	s.operationsMu.RLock()
	defer s.operationsMu.RUnlock()
	return len(s.activeOperations) > 0
}

// GetActiveOperationCount returns the number of active operations
func (s *Server) GetActiveOperationCount() int {
	s.operationsMu.RLock()
	defer s.operationsMu.RUnlock()
	return len(s.activeOperations)
}

// IsOperationExpired checks if any operation has exceeded the server's timeout
func (s *Server) IsOperationExpired() bool {
	s.operationsMu.RLock()
	defer s.operationsMu.RUnlock()

	timeout := time.Duration(s.operationTimeoutSec) * time.Second
	now := time.Now()

	for _, info := range s.activeOperations {
		if now.Sub(info.StartTime) > timeout {
			return true
		}
	}
	return false
}

// GetOperationTimeoutSec returns the server's operation timeout in seconds
func (s *Server) GetOperationTimeoutSec() int {
	return s.operationTimeoutSec
}

// SendMessage sends a JSON-RPC message to the MCP server using the request queue
// This method is deprecated - use SendAndReceive for new code
func (s *Server) SendMessage(message []byte) error {
	// For backward compatibility, we still support this method
	// but we recommend using SendAndReceive for proper request/response correlation
	return s.sendMessageDirect(message)
}

// readMessageDirect reads a message directly (internal use by request processor)
func (s *Server) readMessageDirect(ctx context.Context) ([]byte, error) {
	// CRITICAL FIX: Use dedicated read mutex to prevent concurrent stdout reads
	//
	// This mutex ensures only one goroutine can read from the MCP server's stdout
	// at a time, preventing stdio deadlocks and race conditions that caused
	// "context deadline exceeded" errors during Remote MCP initialization.
	//
	// The mutex must be acquired BEFORE checking if stdout is available to
	// maintain proper synchronization across all read operations.
	//
	// DO NOT REMOVE OR MODIFY - this fixes the core Claude.ai connection issue
	s.readMu.Lock()
	defer s.readMu.Unlock()

	// Safely access stdout and server name under read lock
	s.mu.RLock()
	stdout := s.Stdout
	serverName := s.Name
	s.mu.RUnlock()

	if stdout == nil {
		s.logger.Error("Server %s not running, cannot read message", serverName)
		return nil, fmt.Errorf("server not running")
	}

	// Use a channel to communicate the result from the reading goroutine
	type readResult struct {
		data []byte
		err  error
	}

	// CRITICAL FIX: Use a timeout-aware buffered reader instead of Scanner
	// This prevents indefinite blocking when MCP servers become unresponsive
	resultChan := make(chan readResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("Panic in readMessageDirect goroutine for server %s: %v", s.Name, r)
				resultChan <- readResult{nil, fmt.Errorf("panic in read operation: %v", r)}
			}
		}()

		// Use line-by-line reading with timeout awareness
		reader := bufio.NewReader(stdout)
		line, _, err := reader.ReadLine()
		if err != nil {
			if err == io.EOF {
				s.logger.Debug("EOF reached for server %s", serverName)
				resultChan <- readResult{nil, io.EOF}
			} else {
				s.logger.Error("Read error for server %s: %v", serverName, err)
				resultChan <- readResult{nil, err}
			}
			return
		}

		// Successfully read line
		data := make([]byte, len(line))
		copy(data, line)
		s.logger.Debug("Read message from server %s: %s", serverName, string(data))
		resultChan <- readResult{data, nil}
	}()

	// Wait for either the read to complete or the context to be cancelled
	select {
	case result := <-resultChan:
		if result.err != nil && result.err != io.EOF {
			s.logger.Error("Failed to read message from server %s: %v", serverName, result.err)
		} else if result.err == nil {
			// Message read successfully - no need to log at INFO level
		}
		return result.data, result.err
	case <-ctx.Done():
		s.logger.Warn("readMessageDirect timeout/cancellation for server %s: %v", serverName, ctx.Err())
		return nil, ctx.Err()
	}
}

// ReadMessage reads a JSON-RPC message from the MCP server with context timeout
func (s *Server) ReadMessage(ctx context.Context) ([]byte, error) {
	// CRITICAL FIX: Use dedicated read mutex to prevent concurrent stdout reads
	//
	// This mutex ensures only one goroutine can read from the MCP server's stdout
	// at a time, preventing stdio deadlocks and race conditions that caused
	// "context deadline exceeded" errors during Remote MCP initialization.
	//
	// The mutex must be acquired BEFORE checking if stdout is available to
	// maintain proper synchronization across all read operations.
	//
	// DO NOT REMOVE OR MODIFY - this fixes the core Claude.ai connection issue
	s.readMu.Lock()
	defer s.readMu.Unlock()

	// Safely access stdout and server name under read lock
	s.mu.RLock()
	stdout := s.Stdout
	serverName := s.Name
	s.mu.RUnlock()

	if stdout == nil {
		s.logger.Error("Server %s not running, cannot read message", serverName)
		return nil, fmt.Errorf("server not running")
	}

	// Use a channel to communicate the result from the reading goroutine
	type readResult struct {
		data []byte
		err  error
	}

	// CRITICAL FIX: Use a timeout-aware buffered reader instead of Scanner
	// This prevents indefinite blocking when MCP servers become unresponsive
	resultChan := make(chan readResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("Panic in ReadMessage goroutine for server %s: %v", s.Name, r)
				resultChan <- readResult{nil, fmt.Errorf("panic in read operation: %v", r)}
			}
		}()

		// Use line-by-line reading with timeout awareness
		reader := bufio.NewReader(stdout)
		line, _, err := reader.ReadLine()
		if err != nil {
			if err == io.EOF {
				s.logger.Debug("EOF reached for server %s", serverName)
				resultChan <- readResult{nil, io.EOF}
			} else {
				s.logger.Error("Read error for server %s: %v", serverName, err)
				resultChan <- readResult{nil, err}
			}
			return
		}

		// Successfully read line
		data := make([]byte, len(line))
		copy(data, line)
		s.logger.Debug("Read message from server %s: %s", serverName, string(data))
		resultChan <- readResult{data, nil}
	}()

	// Wait for either the read to complete or the context to be cancelled
	select {
	case result := <-resultChan:
		if result.err != nil && result.err != io.EOF {
			s.logger.Error("Failed to read message from server %s: %v", serverName, result.err)
		} else if result.err == nil {
			// Message read successfully - no need to log at INFO level
		}
		return result.data, result.err
	case <-ctx.Done():
		s.logger.Warn("ReadMessage timeout/cancellation for server %s: %v", serverName, ctx.Err())
		return nil, ctx.Err()
	}
}

// monitor watches the process and handles restarts if needed
func (s *Server) monitor() {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("Panic in monitor goroutine for server %s: %v", s.Name, r)
		}
		s.logger.Info("Monitor goroutine exiting for server %s", s.Name)
	}()

	if s.Process == nil {
		s.logger.Error("No process to monitor for server %s", s.Name)
		return
	}

	s.logger.Info("Starting monitor for server %s (PID: %d)", s.Name, s.Process.Process.Pid)

	// Create a channel to receive the process exit status
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("Panic in process wait goroutine for server %s: %v", s.Name, r)
				done <- fmt.Errorf("panic in process wait: %v", r)
			}
		}()

		err := s.Process.Wait()
		done <- err
	}()

	// Wait for either the process to exit or context to be cancelled
	select {
	case err := <-done:
		if err != nil {
			s.logger.Error("MCP server %s exited with error: %v", s.Name, err)
		} else {
			s.logger.Info("MCP server %s exited cleanly", s.Name)
		}
		// TODO: Implement restart logic here if desired
		return
	case <-s.ctx.Done():
		s.logger.Info("Monitor context cancelled for server %s", s.Name)
		// Process will be terminated by the Stop() method
		return
	}
}

// RestartServer restarts a specific MCP server by name
func (m *Manager) RestartServer(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	server, exists := m.servers[name]
	if !exists {
		return fmt.Errorf("server %s not found", name)
	}

	logger.System().Info("Stopping MCP server %s for restart", name)
	server.Stop()

	// Wait a moment for clean shutdown
	time.Sleep(500 * time.Millisecond)

	logger.System().Info("Restarting MCP server %s", name)
	return m.startServer(name, server.Config)
}
