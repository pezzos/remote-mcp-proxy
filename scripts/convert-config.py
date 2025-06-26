#!/usr/bin/env python3

import json
import sys
import os
import subprocess
import glob

def find_binary_for_package(package_name):
    """Find the binary name for an npm package by reading its package.json"""
    try:
        # Try to find the package installation directory
        npm_global_result = subprocess.run(['npm', 'root', '-g'], 
                                         capture_output=True, text=True, check=True)
        npm_global_root = npm_global_result.stdout.strip()
        
        # Convert package name to directory path
        package_dir = os.path.join(npm_global_root, package_name)
        
        if not os.path.exists(package_dir):
            print(f"Package directory not found: {package_dir}")
            return None
        
        # Read the package.json to find binary definitions
        package_json_path = os.path.join(package_dir, 'package.json')
        if not os.path.exists(package_json_path):
            print(f"package.json not found: {package_json_path}")
            return None
        
        with open(package_json_path, 'r') as f:
            package_data = json.load(f)
        
        # Check for binary definitions
        bin_data = package_data.get('bin', {})
        
        if isinstance(bin_data, str):
            # Single binary case
            binary_name = os.path.basename(package_name)
            if binary_name.startswith('@'):
                binary_name = binary_name.split('/')[-1]
            return binary_name
        elif isinstance(bin_data, dict):
            # Multiple binaries case - return the first one
            if bin_data:
                return list(bin_data.keys())[0]
        
        print(f"No binary found in package.json for {package_name}")
        return None
        
    except Exception as e:
        print(f"Error finding binary for {package_name}: {e}")
        return None

def convert_npx_to_binary(config):
    """Convert npx commands to direct binary calls for pre-installed packages"""
    
    if 'mcpServers' not in config:
        return config
    
    converted_config = {"mcpServers": {}}
    
    for server_name, server_config in config['mcpServers'].items():
        new_server_config = server_config.copy()
        command = server_config.get('command', '')
        args = server_config.get('args', [])
        
        if command == 'npx' and args:
            # Extract package name from npx args
            package_name = None
            remaining_args = []
            
            for i, arg in enumerate(args):
                if arg == '-y':
                    continue  # Skip -y flag
                elif arg.startswith('@') or (not arg.startswith('-') and package_name is None):
                    if not arg.startswith('-'):
                        package_name = arg
                        continue
                
                # Collect remaining args (non-package arguments)
                remaining_args.append(arg)
            
            if package_name:
                # Try to find the binary dynamically
                binary_name = find_binary_for_package(package_name)
                
                if binary_name:
                    new_server_config['command'] = binary_name
                    new_server_config['args'] = remaining_args
                    print(f"Converted {server_name}: npx {package_name} -> {binary_name}")
                else:
                    print(f"Warning: Could not find binary for {package_name}, keeping as npx")
        
        converted_config['mcpServers'][server_name] = new_server_config
    
    return converted_config

def main():
    source_config_file = "/app/config.json"
    target_config_file = "/tmp/config.json"
    
    if not os.path.exists(source_config_file):
        print(f"Error: {source_config_file} not found", file=sys.stderr)
        sys.exit(1)
    
    # Read original config
    with open(source_config_file, 'r') as f:
        original_config = json.load(f)
    
    # Convert npx commands to binary calls
    converted_config = convert_npx_to_binary(original_config)
    
    # Write converted config to writable location
    with open(target_config_file, 'w') as f:
        json.dump(converted_config, f, indent=2)
    
    print(f"Successfully converted config and saved to {target_config_file}")

if __name__ == "__main__":
    main()