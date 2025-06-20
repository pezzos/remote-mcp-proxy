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
func (m *Manager) startServer(name string, cfg config.MCPServer) error {
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
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		cancel()
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

	// Start monitoring the process
	go server.monitor()

	m.servers[name] = server
	log.Printf("Started MCP server: %s (PID: %d)", name, cmd.Process.Pid)

	return nil
}

// Stop gracefully stops the MCP server
func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Process == nil {
		return
	}

	// Cancel context to signal shutdown
	s.cancel()

	// Close stdin to signal the process to exit
	if s.Stdin != nil {
		s.Stdin.Close()
	}

	// Wait for process to exit gracefully
	done := make(chan error, 1)
	go func() {
		done <- s.Process.Wait()
	}()

	select {
	case <-done:
		log.Printf("MCP server %s exited gracefully", s.Name)
	case <-time.After(10 * time.Second):
		// Force kill if graceful shutdown takes too long
		log.Printf("Force killing MCP server %s", s.Name)
		if s.Process.Process != nil {
			s.Process.Process.Signal(syscall.SIGKILL)
		}
	}

	s.Process = nil
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

// ReadMessage reads a JSON-RPC message from the MCP server
func (s *Server) ReadMessage() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Stdout == nil {
		return nil, fmt.Errorf("server not running")
	}

	scanner := bufio.NewScanner(s.Stdout)
	if scanner.Scan() {
		return scanner.Bytes(), nil
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return nil, io.EOF
}

// monitor watches the process and handles restarts if needed
func (s *Server) monitor() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			if s.Process != nil {
				if err := s.Process.Wait(); err != nil {
					log.Printf("MCP server %s exited with error: %v", s.Name, err)
				}
			}
			// Process exited, could implement restart logic here
			return
		}
	}
}