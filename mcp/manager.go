package mcp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"remote-mcp-proxy/config"
)

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
		m.servers[name] = &Server{
			Name:   name,
			Config: cfg,
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
	Name      string `json:"name"`
	Running   bool   `json:"running"`
	PID       int    `json:"pid,omitempty"`
	Command   string `json:"command"`
	Args      []string `json:"args,omitempty"`
	Error     string `json:"error,omitempty"`
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
		log.Printf("Stopping MCP server: %s", name)
		server.Stop()
	}
}

// startServer starts a single MCP server
// NOTE: This method must be called with m.mu locked
func (m *Manager) startServer(name string, cfg config.MCPServer) error {
	log.Printf("INFO: Starting MCP server: %s", name)

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
		log.Printf("ERROR: Failed to create stdin pipe for server %s: %v", name, err)
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		if stdin != nil {
			stdin.Close()
		}
		log.Printf("ERROR: Failed to create stdout pipe for server %s: %v", name, err)
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
		log.Printf("ERROR: Failed to start process for server %s: %v", name, err)
		return fmt.Errorf("failed to start process: %w", err)
	}

	server := &Server{
		Name:    name,
		Config:  cfg,
		Process: cmd,
		Stdin:   stdin,
		Stdout:  stdout,
		ctx:     ctx,
		cancel:  cancel,
	}

	// Update the server in the map (mutex is already held by caller)
	m.servers[name] = server

	// Start monitoring the process
	go server.monitor()

	log.Printf("INFO: Successfully started MCP server %s (PID: %d)", name, cmd.Process.Pid)
	return nil
}

// Stop gracefully stops the MCP server
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("INFO: Stopping MCP server: %s", s.Name)

	if s.Process == nil {
		log.Printf("WARNING: Server %s already stopped or not started", s.Name)
		// Still clean up pipes even if no process
		if s.Stdin != nil {
			if err := s.Stdin.Close(); err != nil {
				log.Printf("ERROR: Failed to close stdin for server %s: %v", s.Name, err)
			}
			s.Stdin = nil
		}
		if s.Stdout != nil {
			if err := s.Stdout.Close(); err != nil {
				log.Printf("ERROR: Failed to close stdout for server %s: %v", s.Name, err)
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
			log.Printf("ERROR: Failed to close stdin for server %s: %v", s.Name, err)
		} else {
			log.Printf("DEBUG: Closed stdin for server %s", s.Name)
		}
		s.Stdin = nil
	}

	if s.Stdout != nil {
		if err := s.Stdout.Close(); err != nil {
			log.Printf("ERROR: Failed to close stdout for server %s: %v", s.Name, err)
		} else {
			log.Printf("DEBUG: Closed stdout for server %s", s.Name)
		}
		s.Stdout = nil
	}

	// Wait for process to exit gracefully
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("ERROR: Panic in process wait for server %s: %v", s.Name, r)
				done <- fmt.Errorf("panic in process wait: %v", r)
			}
		}()

		err := s.Process.Wait()
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			log.Printf("WARNING: MCP server %s exited with error: %v", s.Name, err)
		} else {
			log.Printf("INFO: MCP server %s exited gracefully", s.Name)
		}
	case <-time.After(10 * time.Second):
		// Force kill if graceful shutdown takes too long
		log.Printf("WARNING: Force killing MCP server %s after timeout", s.Name)
		if s.Process.Process != nil {
			if err := s.Process.Process.Signal(syscall.SIGKILL); err != nil {
				log.Printf("ERROR: Failed to kill process for server %s: %v", s.Name, err)
			} else {
				log.Printf("INFO: Sent SIGKILL to server %s", s.Name)
			}
		}

		// Wait a bit more for the forced kill to take effect
		select {
		case err := <-done:
			if err != nil {
				log.Printf("INFO: Server %s terminated after SIGKILL with error: %v", s.Name, err)
			} else {
				log.Printf("INFO: Server %s terminated after SIGKILL", s.Name)
			}
		case <-time.After(2 * time.Second):
			log.Printf("ERROR: Server %s did not respond to SIGKILL", s.Name)
		}
	}

	s.Process = nil
	log.Printf("INFO: Server %s stop completed", s.Name)
}

// SendMessage sends a JSON-RPC message to the MCP server
func (s *Server) SendMessage(message []byte) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Stdin == nil {
		return fmt.Errorf("server not running")
	}

	_, err := s.Stdin.Write(append(message, '\n'))
	return err
}

// ReadMessage reads a JSON-RPC message from the MCP server with context timeout
func (s *Server) ReadMessage(ctx context.Context) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Stdout == nil {
		log.Printf("ERROR: Server %s not running, cannot read message", s.Name)
		return nil, fmt.Errorf("server not running")
	}

	// Use a channel to communicate the result from the reading goroutine
	type readResult struct {
		data []byte
		err  error
	}

	resultChan := make(chan readResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("ERROR: Panic in ReadMessage goroutine for server %s: %v", s.Name, r)
				resultChan <- readResult{nil, fmt.Errorf("panic in read operation: %v", r)}
			}
		}()

		scanner := bufio.NewScanner(s.Stdout)
		if scanner.Scan() {
			data := make([]byte, len(scanner.Bytes()))
			copy(data, scanner.Bytes())
			log.Printf("DEBUG: Read message from server %s: %s", s.Name, string(data))
			resultChan <- readResult{data, nil}
		} else {
			scanErr := scanner.Err()
			if scanErr != nil {
				log.Printf("ERROR: Scanner error for server %s: %v", s.Name, scanErr)
				resultChan <- readResult{nil, scanErr}
			} else {
				log.Printf("DEBUG: EOF reached for server %s", s.Name)
				resultChan <- readResult{nil, io.EOF}
			}
		}
	}()

	// Wait for either the read to complete or the context to be cancelled
	select {
	case result := <-resultChan:
		if result.err != nil && result.err != io.EOF {
			log.Printf("ERROR: Failed to read message from server %s: %v", s.Name, result.err)
		} else if result.err == nil {
			log.Printf("INFO: Successfully read message from server %s", s.Name)
		}
		return result.data, result.err
	case <-ctx.Done():
		log.Printf("WARNING: ReadMessage timeout/cancellation for server %s: %v", s.Name, ctx.Err())
		return nil, ctx.Err()
	}
}

// monitor watches the process and handles restarts if needed
func (s *Server) monitor() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ERROR: Panic in monitor goroutine for server %s: %v", s.Name, r)
		}
		log.Printf("INFO: Monitor goroutine exiting for server %s", s.Name)
	}()

	if s.Process == nil {
		log.Printf("ERROR: No process to monitor for server %s", s.Name)
		return
	}

	log.Printf("INFO: Starting monitor for server %s (PID: %d)", s.Name, s.Process.Process.Pid)

	// Create a channel to receive the process exit status
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("ERROR: Panic in process wait goroutine for server %s: %v", s.Name, r)
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
			log.Printf("ERROR: MCP server %s exited with error: %v", s.Name, err)
		} else {
			log.Printf("INFO: MCP server %s exited cleanly", s.Name)
		}
		// TODO: Implement restart logic here if desired
		return
	case <-s.ctx.Done():
		log.Printf("INFO: Monitor context cancelled for server %s", s.Name)
		// Process will be terminated by the Stop() method
		return
	}
}
