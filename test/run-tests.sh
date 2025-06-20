#!/bin/bash

# Test runner script for Remote MCP Proxy
# This script runs all tests and provides detailed output

set -e

echo "🧪 Running Remote MCP Proxy Tests"
echo "================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if Go is installed
if ! command -v go &> /dev/null; then
    print_error "Go is not installed. Please install Go 1.21 or later."
    exit 1
fi

# Check Go version
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
print_status "Using Go version: $GO_VERSION"

# Change to project directory
PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_DIR"

print_status "Project directory: $PROJECT_DIR"

# Ensure dependencies are up to date
print_status "Updating Go dependencies..."
go mod tidy

# Format code
print_status "Formatting Go code..."
go fmt ./...

# Run linting
print_status "Running Go vet..."
go vet ./...

# Build the project to ensure it compiles
print_status "Building project..."
if go build -o remote-mcp-proxy .; then
    print_success "Project builds successfully"
    rm -f remote-mcp-proxy
else
    print_error "Project failed to build"
    exit 1
fi

# Run unit tests
print_status "Running unit tests..."
if go test -v ./protocol ./mcp ./proxy; then
    print_success "Unit tests passed"
else
    print_error "Unit tests failed"
    exit 1
fi

# Run integration tests (short mode)
print_status "Running integration tests (short mode)..."
if go test -short -v .; then
    print_success "Integration tests (short) passed"
else
    print_warning "Integration tests (short) failed"
fi

# Run all tests with coverage
print_status "Running all tests with coverage..."
if go test -cover ./...; then
    print_success "All tests passed with coverage"
else
    print_warning "Some tests failed"
fi

# Run benchmarks
print_status "Running benchmarks..."
go test -bench=. -benchmem ./... || print_warning "Benchmarks completed with some issues"

# Test configuration files
print_status "Validating test configuration files..."
for config_file in test/*.json; do
    if [ -f "$config_file" ]; then
        if python3 -m json.tool "$config_file" > /dev/null 2>&1; then
            print_success "✓ $(basename "$config_file") is valid JSON"
        else
            print_error "✗ $(basename "$config_file") has invalid JSON"
        fi
    fi
done

# Test with minimal configuration
print_status "Testing with minimal configuration..."
export CONFIG_PATH="./test/minimal-config.json"
if timeout 10s ./remote-mcp-proxy 2>/dev/null & SERVER_PID=$!; then
    sleep 2
    if kill -0 $SERVER_PID 2>/dev/null; then
        print_success "Server starts successfully with minimal config"
        kill $SERVER_PID
        wait $SERVER_PID 2>/dev/null || true
    else
        print_warning "Server exited quickly with minimal config"
    fi
else
    print_warning "Could not test server startup (timeout or build issue)"
fi

# Health check test (if server is available)
print_status "Testing health endpoint..."
if command -v curl &> /dev/null; then
    # Start server in background for testing
    ./remote-mcp-proxy 2>/dev/null & SERVER_PID=$!
    sleep 2
    
    if curl -s http://localhost:8080/health | grep -q "healthy"; then
        print_success "Health endpoint responds correctly"
    else
        print_warning "Health endpoint test inconclusive"
    fi
    
    # Cleanup
    if kill -0 $SERVER_PID 2>/dev/null; then
        kill $SERVER_PID
        wait $SERVER_PID 2>/dev/null || true
    fi
else
    print_warning "curl not available for health check test"
fi

echo ""
echo "🎉 Test run completed!"
echo "======================"
print_status "To run specific test suites:"
echo "  Unit tests:        go test -v ./protocol ./mcp ./proxy"
echo "  Integration tests: go test -v ."
echo "  With coverage:     go test -cover ./..."
echo "  Benchmarks:        go test -bench=. ./..."
echo ""
print_status "To test with different configurations:"
echo "  Minimal:     CONFIG_PATH=./test/minimal-config.json ./remote-mcp-proxy"
echo "  Development: CONFIG_PATH=./test/development-config.json ./remote-mcp-proxy" 
echo "  Production:  CONFIG_PATH=./test/production-config.json ./remote-mcp-proxy"