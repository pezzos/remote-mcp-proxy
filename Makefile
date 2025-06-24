.PHONY: generate up build down clean help install-deps

# Check if gomplate is available (local or system-wide)
check-deps:
	@(which gomplate > /dev/null || test -x /tmp/gomplate) || (echo "Error: gomplate not found. Install with:" && \
		echo "  make install-deps" && \
		exit 1)

# Install gomplate (for Linux) to /tmp
install-deps:
	@echo "Installing gomplate to /tmp..."
	@wget -q -O /tmp/gomplate https://github.com/hairyhenderson/gomplate/releases/download/v3.11.5/gomplate_linux-amd64
	@chmod +x /tmp/gomplate
	@echo "gomplate installed successfully to /tmp/gomplate"

# Generate docker-compose.yml from template and config.json
docker-compose.yml: config.json docker-compose.yml.template check-deps
	@echo "Generating docker-compose.yml from config.json..."
	@(which gomplate > /dev/null && gomplate -d config=config.json -f docker-compose.yml.template -o docker-compose.yml) || \
	 (test -x /tmp/gomplate && /tmp/gomplate -d config=config.json -f docker-compose.yml.template -o docker-compose.yml)
	@echo "Generated docker-compose.yml with MCP servers: $$(jq -r '.mcpServers | keys | join(", ")' config.json)"

# Generate docker-compose.yml
generate: docker-compose.yml

# Build the Docker image (generates compose file first)
build: generate
	@echo "Building Docker images..."
	docker-compose build

# Start services (generates compose file and builds if needed)
up: generate
	@echo "Starting services..."
	docker-compose up --build -d

# Stop and remove services
down:
	@if [ -f docker-compose.yml ]; then \
		echo "Stopping services..."; \
		docker-compose down; \
	else \
		echo "No docker-compose.yml found, nothing to stop"; \
	fi

# Remove generated files
clean:
	@echo "Cleaning generated files..."
	@rm -f docker-compose.yml
	@echo "Cleaned docker-compose.yml"

# Show logs
logs:
	@if [ -f docker-compose.yml ]; then \
		docker-compose logs -f; \
	else \
		echo "No docker-compose.yml found, run 'make generate' first"; \
	fi

# Restart services
restart: down up

# Show help
help:
	@echo "Remote MCP Proxy - Dynamic Docker Compose Generation"
	@echo ""
	@echo "Usage:"
	@echo "  make install-deps  Install gomplate dependency (Linux)"
	@echo "  make generate      Generate docker-compose.yml from config.json"
	@echo "  make build         Build Docker images"
	@echo "  make up            Start services (generates compose + builds)"
	@echo "  make down          Stop services"
	@echo "  make restart       Restart services"
	@echo "  make logs          Show service logs"
	@echo "  make clean         Remove generated docker-compose.yml"
	@echo "  make help          Show this help"
	@echo ""
	@echo "The system will automatically generate Traefik routes for each MCP server"
	@echo "defined in config.json. Current servers:"
	@if [ -f config.json ]; then \
		jq -r '.mcpServers | keys[]' config.json | sed 's/^/  - /'; \
	else \
		echo "  (config.json not found)"; \
	fi

# Default target
.DEFAULT_GOAL := help