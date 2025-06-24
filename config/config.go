package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// MCPServer represents a single MCP server configuration
type MCPServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// Config represents the entire configuration file
type Config struct {
	MCPServers map[string]MCPServer `json:"mcpServers"`
	// Environment-based configuration (loaded from env vars)
	Domain string `json:"-"` // Domain for subdomain routing
	Port   string `json:"-"` // HTTP server port
}

// Load reads and parses the configuration file
func Load(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Load environment variables
	config.LoadEnvironmentConfig()

	return &config, nil
}

// validate checks that the configuration is valid
func (c *Config) validate() error {
	if len(c.MCPServers) == 0 {
		return fmt.Errorf("no MCP servers configured")
	}

	for name, server := range c.MCPServers {
		if server.Command == "" {
			return fmt.Errorf("server %s: command cannot be empty", name)
		}
	}

	return nil
}

// LoadEnvironmentConfig loads configuration from environment variables
func (c *Config) LoadEnvironmentConfig() {
	// Domain configuration for subdomain routing
	if domain := os.Getenv("MCP_DOMAIN"); domain != "" {
		c.Domain = domain
	} else if domain := os.Getenv("DOMAIN"); domain != "" {
		c.Domain = domain
	} else {
		c.Domain = "localhost" // Default for development
	}

	// Port configuration
	if port := os.Getenv("PORT"); port != "" {
		c.Port = port
	} else {
		c.Port = "8080" // Default port
	}
}

// GetDomain returns the configured domain for subdomain routing
func (c *Config) GetDomain() string {
	return c.Domain
}

// GetPort returns the configured HTTP server port
func (c *Config) GetPort() string {
	return c.Port
}

// ValidateSubdomain checks if a subdomain matches the expected format for MCP servers
func (c *Config) ValidateSubdomain(host string) (string, bool) {
	// Expected format: {server}.mcp.{domain}
	expectedSuffix := fmt.Sprintf(".mcp.%s", c.Domain)
	
	// Remove port if present
	for idx := 0; idx < len(host); idx++ {
		if host[idx] == ':' {
			host = host[:idx]
			break
		}
	}
	
	// Check if host ends with expected suffix
	if len(host) <= len(expectedSuffix) {
		return "", false
	}
	
	if host[len(host)-len(expectedSuffix):] != expectedSuffix {
		return "", false
	}
	
	// Extract server name
	serverName := host[:len(host)-len(expectedSuffix)]
	
	// Validate server name exists in configuration
	if _, exists := c.MCPServers[serverName]; !exists {
		return "", false
	}
	
	return serverName, true
}
