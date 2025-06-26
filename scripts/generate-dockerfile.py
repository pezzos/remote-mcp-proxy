#!/usr/bin/env python3

import json
import sys
import os

def extract_packages_from_config(config):
    """Extract package names dynamically from config.json"""
    packages = {
        'npm': set(),
        'pip': set(),
        'uv': set()
    }
    
    if 'mcpServers' not in config:
        return packages
    
    for server_name, server_config in config['mcpServers'].items():
        command = server_config.get('command', '')
        args = server_config.get('args', [])
        
        if command == 'npx' and args:
            # Parse npx args: ["−y", "@package/name", ...other args]
            package_name = None
            for i, arg in enumerate(args):
                if arg.startswith('@') or (not arg.startswith('-') and i > 0):
                    # Found package name (starts with @ or first non-flag arg)
                    if not arg.startswith('-'):
                        package_name = arg
                        break
            
            if package_name:
                packages['npm'].add(package_name)
                print(f"Found npm package for {server_name}: {package_name}")
        
        elif command == 'uvx' and args:
            # Parse uvx args: similar to npx but for Python packages
            package_name = None
            for i, arg in enumerate(args):
                if not arg.startswith('-') and i >= 0:
                    package_name = arg
                    break
            
            if package_name:
                packages['uv'].add(package_name)
                print(f"Found uv package for {server_name}: {package_name}")
        
        elif command == 'python' and args:
            # Parse python -m package calls
            if len(args) >= 2 and args[0] == '-m':
                package_name = args[1]
                packages['pip'].add(package_name)
                print(f"Found pip package for {server_name}: {package_name}")
        
        else:
            # Direct binary commands - skip as they should be pre-installed
            print(f"Skipping direct command for {server_name}: {command}")
    
    return packages

def generate_install_commands(packages):
    """Generate install commands for different package managers"""
    install_lines = []
    
    # npm packages
    if packages['npm']:
        npm_packages = sorted(packages['npm'])
        for package in npm_packages:
            install_lines.append(f" && npm install -g {package}")
    
    # pip packages
    if packages['pip']:
        pip_packages = sorted(packages['pip'])
        for package in pip_packages:
            install_lines.append(f" && pip3 install --no-cache-dir --break-system-packages {package}")
    
    # uv packages  
    if packages['uv']:
        uv_packages = sorted(packages['uv'])
        for package in uv_packages:
            install_lines.append(f" && uv tool install {package}")
    
    return install_lines

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
    
    # Read config.json
    with open(config_file, 'r') as f:
        config = json.load(f)
    
    # Extract packages dynamically
    packages = extract_packages_from_config(config)
    
    # Generate install commands
    install_lines = generate_install_commands(packages)
    
    # Read template
    with open(template_file, 'r') as f:
        template_content = f.readlines()
    
    # Process template
    output_lines = []
    for line in template_content:
        if "{{range .MCPPackages}} && npm install -g {{.}}{{end}}" in line:
            # Replace with actual install commands
            if install_lines:
                install_commands = " \\\n".join(install_lines)
                line = line.replace("{{range .MCPPackages}} && npm install -g {{.}}{{end}}", install_commands)
            else:
                # Skip this line if no packages
                continue
        output_lines.append(line)
    
    # Write output
    with open(output_file, 'w') as f:
        f.writelines(output_lines)
    
    all_packages = list(packages['npm']) + list(packages['pip']) + list(packages['uv'])
    print(f"Generating {output_file} with packages: {' '.join(all_packages)}")
    print(f"Successfully generated {output_file}")

if __name__ == "__main__":
    main()