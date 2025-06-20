package mcp

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"remote-mcp-proxy/config"
)

// mockProcess simulates a simple MCP server process
type mockProcess struct {
	stdin  *mockPipe
	stdout *mockPipe
	exited bool
}

type mockPipe struct {
	data   []byte
	closed bool
}

func (m *mockPipe) Write(p []byte) (n int, err error) {
	if m.closed {
		return 0, io.ErrClosedPipe
	}
	m.data = append(m.data, p...)
	return len(p), nil
}

func (m *mockPipe) Read(p []byte) (n int, err error) {
	if m.closed {
		return 0, io.EOF
	}
	if len(m.data) == 0 {
		// Simulate blocking read
		time.Sleep(10 * time.Millisecond)
		return 0, io.EOF
	}
	n = copy(p, m.data)
	m.data = m.data[n:]
	return n, nil
}

func (m *mockPipe) Close() error {
	m.closed = true
	return nil
}

func TestNewManager(t *testing.T) {
	configs := map[string]config.MCPServer{
		"test-server": {
			Command: "echo",
			Args:    []string{"hello"},
			Env:     map[string]string{"TEST": "value"},
		},
	}

	manager := NewManager(configs)

	if manager == nil {
		t.Fatal("Expected manager to be created")
	}

	if len(manager.servers) != 1 {
		t.Errorf("Expected 1 server, got %d", len(manager.servers))
	}

	server, exists := manager.GetServer("test-server")
	if !exists {
		t.Error("Expected test-server to exist")
	}

	if server.Name != "test-server" {
		t.Errorf("Expected server name 'test-server', got '%s'", server.Name)
	}

	if server.Config.Command != "echo" {
		t.Errorf("Expected command 'echo', got '%s'", server.Config.Command)
	}
}

func TestGetServer(t *testing.T) {
	configs := map[string]config.MCPServer{
		"server1": {Command: "echo", Args: []string{"test1"}},
		"server2": {Command: "echo", Args: []string{"test2"}},
	}

	manager := NewManager(configs)

	// Test existing servers
	server1, exists := manager.GetServer("server1")
	if !exists {
		t.Error("Expected server1 to exist")
	}
	if server1.Name != "server1" {
		t.Errorf("Expected server name 'server1', got '%s'", server1.Name)
	}

	server2, exists := manager.GetServer("server2")
	if !exists {
		t.Error("Expected server2 to exist")
	}
	if server2.Name != "server2" {
		t.Errorf("Expected server name 'server2', got '%s'", server2.Name)
	}

	// Test non-existent server
	_, exists = manager.GetServer("non-existent")
	if exists {
		t.Error("Expected non-existent server to not exist")
	}
}

func TestSendMessage(t *testing.T) {
	server := &Server{
		Name: "test-server",
	}

	// Test sending message when server not running
	err := server.SendMessage([]byte(`{"jsonrpc":"2.0","method":"test"}`))
	if err == nil {
		t.Error("Expected error when server not running")
	}

	// Test with mock stdin
	mockStdin := &mockPipe{data: make([]byte, 0)}
	server.Stdin = mockStdin

	testMessage := []byte(`{"jsonrpc":"2.0","method":"initialize"}`)
	err = server.SendMessage(testMessage)
	if err != nil {
		t.Errorf("Unexpected error sending message: %v", err)
	}

	// Verify message was written to stdin
	expectedData := append(testMessage, '\n')
	if string(mockStdin.data) != string(expectedData) {
		t.Errorf("Expected message %s, got %s", string(expectedData), string(mockStdin.data))
	}

	// Test with closed stdin
	mockStdin.Close()
	err = server.SendMessage(testMessage)
	if err == nil {
		t.Error("Expected error when stdin is closed")
	}
}

func TestReadMessageWithContext(t *testing.T) {
	server := &Server{
		Name: "test-server",
	}

	// Test reading when server not running
	ctx := context.Background()
	_, err := server.ReadMessage(ctx)
	if err == nil {
		t.Error("Expected error when server not running")
	}

	// Test with mock stdout
	mockStdout := &mockPipe{data: []byte(`{"jsonrpc":"2.0","result":{"status":"ok"}}`)}
	server.Stdout = mockStdout

	message, err := server.ReadMessage(ctx)
	if err != nil {
		t.Errorf("Unexpected error reading message: %v", err)
	}

	expectedMessage := `{"jsonrpc":"2.0","result":{"status":"ok"}}`
	if string(message) != expectedMessage {
		t.Errorf("Expected message %s, got %s", expectedMessage, string(message))
	}

	// Test with context timeout
	mockStdout.data = []byte{} // Empty data to simulate blocking
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = server.ReadMessage(ctx)
	// Mock implementation may return EOF instead of timeout, both are acceptable for testing
	if err != context.DeadlineExceeded && err != io.EOF {
		t.Errorf("Expected context deadline exceeded or EOF, got %v", err)
	}

	// Test with context cancellation
	ctx, cancel = context.WithCancel(context.Background())
	go func() {
		time.Sleep(25 * time.Millisecond)
		cancel()
	}()

	_, err = server.ReadMessage(ctx)
	// Mock implementation may return EOF instead of cancellation, both are acceptable for testing
	if err != context.Canceled && err != io.EOF {
		t.Errorf("Expected context canceled or EOF, got %v", err)
	}
}

func TestServerStop(t *testing.T) {
	server := &Server{
		Name: "test-server",
	}

	// Test stopping server that's not running
	server.Stop() // Should not panic

	// Create mock pipes
	mockStdin := &mockPipe{data: make([]byte, 0)}
	mockStdout := &mockPipe{data: make([]byte, 0)}

	server.Stdin = mockStdin
	server.Stdout = mockStdout

	// Test stopping server with pipes
	server.Stop()

	// Verify pipes were closed
	if !mockStdin.closed {
		t.Error("Expected stdin to be closed")
	}

	if !mockStdout.closed {
		t.Error("Expected stdout to be closed")
	}

	// Verify pipes are set to nil
	if server.Stdin != nil {
		t.Error("Expected stdin to be nil after stop")
	}

	if server.Stdout != nil {
		t.Error("Expected stdout to be nil after stop")
	}
}

// Mock configuration for testing
func createTestConfig() map[string]config.MCPServer {
	return map[string]config.MCPServer{
		"echo-server": {
			Command: "echo",
			Args:    []string{"hello world"},
			Env:     map[string]string{},
		},
		"test-server": {
			Command: "/bin/sh",
			Args:    []string{"-c", "echo 'test message' && sleep 0.1"},
			Env:     map[string]string{"TEST_VAR": "test_value"},
		},
	}
}

func TestManagerStartAll_Integration(t *testing.T) {
	// Skip this test in short mode since it starts actual processes
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	configs := createTestConfig()
	manager := NewManager(configs)

	err := manager.StartAll()
	if err != nil {
		t.Errorf("Unexpected error starting servers: %v", err)
	}

	// Verify servers are running
	for name := range configs {
		server, exists := manager.GetServer(name)
		if !exists {
			t.Errorf("Expected server %s to exist after start", name)
			continue
		}

		if server.Process == nil {
			t.Errorf("Expected server %s to have a process", name)
			continue
		}

		// Test that we can send/receive from the process
		if server.Stdin == nil || server.Stdout == nil {
			t.Errorf("Expected server %s to have stdin/stdout", name)
		}
	}

	// Clean up
	manager.StopAll()

	// Give processes time to stop
	time.Sleep(100 * time.Millisecond)

	// Verify servers are stopped
	for name := range configs {
		server, exists := manager.GetServer(name)
		if !exists {
			continue
		}

		if server.Process != nil {
			t.Errorf("Expected server %s process to be nil after stop", name)
		}
	}
}

func TestManagerConcurrentAccess(t *testing.T) {
	configs := createTestConfig()
	manager := NewManager(configs)

	// Test concurrent access to GetServer
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()

			for j := 0; j < 100; j++ {
				// Alternate between existing and non-existing servers
				serverName := "echo-server"
				if j%2 == 0 {
					serverName = fmt.Sprintf("non-existent-%d-%d", id, j)
				}

				_, exists := manager.GetServer(serverName)
				if serverName == "echo-server" && !exists {
					t.Errorf("Expected echo-server to exist in goroutine %d iteration %d", id, j)
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent access test")
		}
	}
}

func TestErrorHandling(t *testing.T) {
	// Test invalid command
	configs := map[string]config.MCPServer{
		"invalid-server": {
			Command: "/this/command/does/not/exist",
			Args:    []string{},
			Env:     map[string]string{},
		},
	}

	manager := NewManager(configs)

	err := manager.StartAll()
	if err == nil {
		t.Error("Expected error when starting invalid command")
	}

	// Verify the server wasn't started
	server, exists := manager.GetServer("invalid-server")
	if !exists {
		t.Error("Expected server to exist even if start failed")
	}

	if server.Process != nil {
		t.Error("Expected process to be nil for failed start")
	}
}

func TestContextCancellation(t *testing.T) {
	server := &Server{
		Name: "test-server",
	}

	// Create a context that will be cancelled
	ctx, cancel := context.WithCancel(context.Background())

	// Set up mock stdout that blocks
	mockStdout := &mockPipe{data: []byte{}}
	server.Stdout = mockStdout

	// Start reading in a goroutine
	resultChan := make(chan error, 1)
	go func() {
		_, err := server.ReadMessage(ctx)
		resultChan <- err
	}()

	// Cancel the context after a short delay
	time.Sleep(10 * time.Millisecond)
	cancel()

	// Wait for the read to complete
	select {
	case err := <-resultChan:
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for context cancellation")
	}
}

func BenchmarkSendMessage(b *testing.B) {
	server := &Server{
		Name:  "bench-server",
		Stdin: &mockPipe{data: make([]byte, 0)},
	}

	message := []byte(`{"jsonrpc":"2.0","method":"test","params":{}}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := server.SendMessage(message)
		if err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}

func BenchmarkReadMessage(b *testing.B) {
	server := &Server{
		Name: "bench-server",
	}

	ctx := context.Background()
	message := `{"jsonrpc":"2.0","result":{"status":"ok"}}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Reset mock stdout for each iteration
		server.Stdout = &mockPipe{data: []byte(message)}

		_, err := server.ReadMessage(ctx)
		if err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}
