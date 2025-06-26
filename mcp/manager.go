package mcp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
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
}

// Manager manages multiple MCP server processes
type Manager struct {
	servers map[string]*Server
	mu      sync.RWMutex
}

// NewManager creates a new MCP manager
func NewManager(configs map[string]config.MCPServer) *Manager {
	m := &Manager{
		servers: make(map[string]*Server),
	}

	// Initialize servers from configs
	for name, cfg := range configs {
		// Get MCP logger for this server
		mcpLogger, err := logger.MCP(name)
		if err != nil {
			// Fallback to system logger if MCP logger fails
			logger.System().Error("Failed to create MCP logger for %s: %v", name, err)
			mcpLogger = logger.System()
		}

		m.servers[name] = &Server{
			Name:         name,
			Config:       cfg,
			requestQueue: make(chan RequestResponse, 100), // Buffer for concurrent requests
			queueStarted: false,
			logger:       mcpLogger,
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

// GetServer returns a server by name
func (m *Manager) GetServer(name string) (*Server, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	server, exists := m.servers[name]
	return server, exists
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
	// Removed redundant server name logging - server context already available in MCP logs

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
