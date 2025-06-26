#!/bin/bash

set -e

echo "Starting Remote MCP Proxy..."

# Convert npx commands to direct binary calls for better performance
echo "Converting config.json from npx to binary commands..."
python3 /app/scripts/convert-config.py

# Set the config file path for the main application
export CONFIG_FILE="/tmp/config.json"

# Start the main application
echo "Starting main application with converted config..."
exec ./main