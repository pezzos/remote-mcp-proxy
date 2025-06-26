package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"remote-mcp-proxy/config"
	"remote-mcp-proxy/mcp"
)

func TestSubdomainMiddleware(t *testing.T) {
	// Create test configuration
	cfg := &config.Config{
		Domain: "example.com",
	}
	cfg.MCPServers = map[string]config.MCPServer{
		"memory": {
			Command: "echo",
			Args:    []string{"test"},
		},
		"sequential-thinking": {
			Command: "echo",
			Args:    []string{"test"},
		},
	}

	// Create test server
	manager := mcp.NewManager(cfg.MCPServers)
	server := NewServerWithConfig(manager, cfg, nil, nil)

	tests := []struct {
		name           string
		host           string
		expectedServer string
		expectError    bool
	}{
		{
			name:           "Valid memory subdomain",
			host:           "memory.mcp.example.com",
			expectedServer: "memory",
			expectError:    false,
		},
		{
			name:           "Valid sequential-thinking subdomain",
			host:           "sequential-thinking.mcp.example.com",
			expectedServer: "sequential-thinking",
			expectError:    false,
		},
		{
			name:           "Valid subdomain with port",
			host:           "memory.mcp.example.com:8080",
			expectedServer: "memory",
			expectError:    false,
		},
		{
			name:        "Invalid subdomain format",
			host:        "memory.example.com",
			expectError: true,
		},
		{
			name:        "Wrong mcp position",
			host:        "mcp.memory.example.com",
			expectError: true,
		},
		{
			name:        "Non-existent server",
			host:        "nonexistent.mcp.example.com",
			expectError: true,
		},
		{
			name:        "Empty host",
			host:        "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/sse", nil)
			req.Host = tt.host

			// Create response recorder
			recorder := httptest.NewRecorder()

			// Create test handler to capture context
			var capturedServer string
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if serverName, ok := r.Context().Value("mcpServer").(string); ok {
					capturedServer = serverName
				}
				w.WriteHeader(http.StatusOK)
			})

			// Apply middleware
			middlewareHandler := server.subdomainMiddleware(testHandler)
			middlewareHandler.ServeHTTP(recorder, req)

			if tt.expectError {
				if capturedServer != "" {
					t.Errorf("Expected no server extraction, but got: %s", capturedServer)
				}
			} else {
				if capturedServer != tt.expectedServer {
					t.Errorf("Expected server %s, got %s", tt.expectedServer, capturedServer)
				}
			}
		})
	}
}

func TestSubdomainRouting(t *testing.T) {
	// Create test configuration
	cfg := &config.Config{
		Domain: "example.com",
	}
	cfg.MCPServers = map[string]config.MCPServer{
		"memory": {
			Command: "echo",
			Args:    []string{"test"},
		},
	}

	// Create test server
	manager := mcp.NewManager(cfg.MCPServers)
	server := NewServerWithConfig(manager, cfg, nil, nil)
	router := server.Router()

	tests := []struct {
		name           string
		host           string
		path           string
		method         string
		expectedStatus int
	}{
		{
			name:           "Valid subdomain SSE endpoint",
			host:           "memory.mcp.example.com",
			path:           "/sse",
			method:         "GET",
			expectedStatus: http.StatusUnauthorized, // Expected due to auth requirement
		},
		{
			name:           "Valid subdomain session endpoint",
			host:           "memory.mcp.example.com",
			path:           "/sessions/test123",
			method:         "POST",
			expectedStatus: http.StatusUnauthorized, // Expected due to auth requirement
		},
		{
			name:           "Invalid subdomain",
			host:           "nonexistent.mcp.example.com",
			path:           "/sse",
			method:         "GET",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Health endpoint on any host",
			host:           "memory.mcp.example.com",
			path:           "/health",
			method:         "GET",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Main domain endpoints",
			host:           "mcp.example.com",
			path:           "/listmcp",
			method:         "GET",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Host = tt.host

			// Create response recorder
			recorder := httptest.NewRecorder()

			// Execute request
			router.ServeHTTP(recorder, req)

			if recorder.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, recorder.Code)
			}
		})
	}
}

func TestConfigValidateSubdomain(t *testing.T) {
	cfg := &config.Config{
		Domain: "example.com",
	}
	cfg.MCPServers = map[string]config.MCPServer{
		"memory": {
			Command: "echo",
			Args:    []string{"test"},
		},
		"sequential-thinking": {
			Command: "echo",
			Args:    []string{"test"},
		},
	}

	tests := []struct {
		name           string
		host           string
		expectedServer string
		expectedValid  bool
	}{
		{
			name:           "Valid memory subdomain",
			host:           "memory.mcp.example.com",
			expectedServer: "memory",
			expectedValid:  true,
		},
		{
			name:           "Valid subdomain with port",
			host:           "memory.mcp.example.com:8080",
			expectedServer: "memory",
			expectedValid:  true,
		},
		{
			name:          "Invalid format",
			host:          "memory.example.com",
			expectedValid: false,
		},
		{
			name:          "Non-existent server",
			host:          "nonexistent.mcp.example.com",
			expectedValid: false,
		},
		{
			name:          "Wrong domain",
			host:          "memory.mcp.wrongdomain.com",
			expectedValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serverName, valid := cfg.ValidateSubdomain(tt.host)

			if valid != tt.expectedValid {
				t.Errorf("Expected valid=%t, got valid=%t", tt.expectedValid, valid)
			}

			if tt.expectedValid && serverName != tt.expectedServer {
				t.Errorf("Expected server %s, got %s", tt.expectedServer, serverName)
			}
		})
	}
}

func TestEnvironmentConfiguration(t *testing.T) {
	tests := []struct {
		name           string
		envVars        map[string]string
		expectedDomain string
		expectedPort   string
	}{
		{
			name: "MCP_DOMAIN takes precedence",
			envVars: map[string]string{
				"MCP_DOMAIN": "mcp-test.com",
				"DOMAIN":     "test.com",
			},
			expectedDomain: "mcp-test.com",
			expectedPort:   "8080",
		},
		{
			name: "DOMAIN fallback",
			envVars: map[string]string{
				"DOMAIN": "test.com",
			},
			expectedDomain: "test.com",
			expectedPort:   "8080",
		},
		{
			name: "Custom port",
			envVars: map[string]string{
				"DOMAIN": "test.com",
				"PORT":   "3000",
			},
			expectedDomain: "test.com",
			expectedPort:   "3000",
		},
		{
			name:           "Defaults",
			envVars:        map[string]string{},
			expectedDomain: "localhost",
			expectedPort:   "8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear and set environment variables
			for key := range tt.envVars {
				t.Setenv(key, tt.envVars[key])
			}

			// Create config and load environment
			cfg := &config.Config{}
			cfg.MCPServers = map[string]config.MCPServer{
				"test": {Command: "echo", Args: []string{"test"}},
			}
			cfg.LoadEnvironmentConfig()

			if cfg.GetDomain() != tt.expectedDomain {
				t.Errorf("Expected domain %s, got %s", tt.expectedDomain, cfg.GetDomain())
			}

			if cfg.GetPort() != tt.expectedPort {
				t.Errorf("Expected port %s, got %s", tt.expectedPort, cfg.GetPort())
			}
		})
	}
}
