package main

import (
	"encoding/json"
	"fmt"
	"log"
	"remote-mcp-proxy/protocol"
)

func main() {
	// Test enhanced capabilities response
	translator := protocol.NewTranslator()
	sessionID := "test-capabilities-session"

	params := protocol.InitializeParams{
		ProtocolVersion: protocol.MCPProtocolVersion,
		Capabilities:    map[string]interface{}{},
		ClientInfo: protocol.ClientInfo{
			Name:    "capabilities-test-client",
			Version: "1.0.0",
		},
	}

	result, err := translator.HandleInitialize(sessionID, params)
	if err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}

	// Convert to JSON for inspection
	capabilitiesJSON, err := json.MarshalIndent(result.Capabilities, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal capabilities: %v", err)
	}

	fmt.Println("Enhanced Capabilities Response:")
	fmt.Println("===============================")
	fmt.Printf("Protocol Version: %s\n", result.ProtocolVersion)
	fmt.Printf("Server: %s v%s\n", result.ServerInfo.Name, result.ServerInfo.Version)
	fmt.Println("\nCapabilities:")
	fmt.Println(string(capabilitiesJSON))

	// Verify specific capability indicators
	fmt.Println("\nCapability Validation:")
	fmt.Println("======================")

	// Check tools capability
	if tools, exists := result.Capabilities["tools"]; exists {
		if toolsMap, ok := tools.(map[string]interface{}); ok {
			if listChanged, hasListChanged := toolsMap["listChanged"]; hasListChanged {
				if listChanged == true {
					fmt.Println("✅ tools.listChanged: true (Claude.ai tool discovery enabled)")
				} else {
					fmt.Printf("❌ tools.listChanged: %v (should be true)\n", listChanged)
				}
			} else {
				fmt.Println("❌ tools.listChanged: missing (Claude.ai won't discover tools)")
			}
		} else {
			fmt.Printf("❌ tools capability malformed: %+v\n", tools)
		}
	} else {
		fmt.Println("❌ tools capability missing")
	}

	// Check resources capability
	if resources, exists := result.Capabilities["resources"]; exists {
		if resourcesMap, ok := resources.(map[string]interface{}); ok {
			if listChanged, hasListChanged := resourcesMap["listChanged"]; hasListChanged {
				if listChanged == true {
					fmt.Println("✅ resources.listChanged: true (resource discovery enabled)")
				} else {
					fmt.Printf("❌ resources.listChanged: %v (should be true)\n", listChanged)
				}
			} else {
				fmt.Println("❌ resources.listChanged: missing")
			}
		}
	} else {
		fmt.Println("❌ resources capability missing")
	}

	// Check prompts capability
	if prompts, exists := result.Capabilities["prompts"]; exists {
		if promptsMap, ok := prompts.(map[string]interface{}); ok {
			if listChanged, hasListChanged := promptsMap["listChanged"]; hasListChanged {
				if listChanged == true {
					fmt.Println("✅ prompts.listChanged: true (prompt discovery enabled)")
				} else {
					fmt.Printf("❌ prompts.listChanged: %v (should be true)\n", listChanged)
				}
			} else {
				fmt.Println("❌ prompts.listChanged: missing")
			}
		}
	} else {
		fmt.Println("❌ prompts capability missing")
	}

	fmt.Println("\nTest completed - Enhanced capabilities should enable tool discovery in Claude.ai")
}
