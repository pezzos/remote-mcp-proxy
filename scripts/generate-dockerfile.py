#!/usr/bin/env python3

import json
import sys
import os

def main():
    # File paths
    config_file = "config.json"
    template_file = "Dockerfile.template"
    output_file = "Dockerfile"
    
    # Check if required files exist
    if not os.path.exists(config_file):
        print(f"Error: {config_file} not found", file=sys.stderr)
        sys.exit(1)
    
    if not os.path.exists(template_file):
        print(f"Error: {template_file} not found", file=sys.stderr)
        sys.exit(1)
    
    # Command to package mapping
    command_to_package = {
        "mcp-server-memory": "@modelcontextprotocol/server-memory",
        "mcp-server-sequential-thinking": "@modelcontextprotocol/server-sequential-thinking",
        "mcp-server-filesystem": "@modelcontextprotocol/server-filesystem",
        "notion-mcp-server": "@notionhq/notion-mcp-server"
    }
    
    # Read config.json
    with open(config_file, 'r') as f:
        config = json.load(f)
    
    # Extract unique commands
    commands = set()
    if 'mcpServers' in config:
        for server_config in config['mcpServers'].values():
            if 'command' in server_config:
                commands.add(server_config['command'])
    
    # Build MCP packages list
    mcp_packages = []
    for command in sorted(commands):
        if command in command_to_package:
            package = command_to_package[command]
            mcp_packages.append(package)
            print(f"Found MCP package mapping: {command} -> {package}")
        else:
            print(f"Warning: No package mapping found for command: {command}")
    
    # Read template
    with open(template_file, 'r') as f:
        template_content = f.readlines()
    
    # Process template
    output_lines = []
    for line in template_content:
        if "{{range .MCPPackages}} && npm install -g {{.}}{{end}}" in line:
            # Replace with actual npm install commands
            if mcp_packages:
                install_commands = " \\\n".join([f" && npm install -g {pkg}" for pkg in mcp_packages])
                line = line.replace("{{range .MCPPackages}} && npm install -g {{.}}{{end}}", install_commands)
            else:
                # Skip this line if no packages
                continue
        output_lines.append(line)
    
    # Write output
    with open(output_file, 'w') as f:
        f.writelines(output_lines)
    
    print(f"Generating {output_file} with MCP packages: {' '.join(mcp_packages)}")
    print(f"Successfully generated {output_file}")

if __name__ == "__main__":
    main()