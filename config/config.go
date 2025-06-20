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
