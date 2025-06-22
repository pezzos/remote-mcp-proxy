# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Final stage
FROM alpine:latest

# Install dependencies for various MCP server types
RUN apk --no-cache add \
    ca-certificates \
    nodejs npm \
    python3 py3-pip \
    curl wget \
    git \
    bash \
    build-base \
    sqlite \
    jq

# Update npm to latest version to ensure npx is available
RUN npm install -g npm@latest

# Install uv (fast Python package installer/manager)
RUN curl -LsSf https://astral.sh/uv/install.sh | sh
ENV PATH="/root/.local/bin:/root/.cargo/bin:$PATH"

# Create uv symlink for easier access
RUN ln -sf /root/.local/bin/uv /usr/local/bin/uv 2>/dev/null || true

# Install essential Python tools that MCP servers commonly need
# Use --break-system-packages for Docker container environment
RUN pip3 install --no-cache-dir --break-system-packages \
    requests \
    httpx \
    pydantic

# Install essential Node.js packages for MCP servers
RUN npm install -g typescript

# Create symlinks for common Python commands
RUN ln -sf /usr/bin/python3 /usr/bin/python

# Verify essential tools and create npx fallback if needed
RUN node --version && npm --version && python --version
RUN npx --help >/dev/null 2>&1 || (echo '#!/bin/sh\nexec npm exec -- "$@"' > /usr/local/bin/npx && chmod +x /usr/local/bin/npx)
RUN npx --version
RUN echo "Build completed successfully"

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/main .

# Create directories for config (config.json will be mounted at runtime)
RUN mkdir -p /app /config

# Expose port
EXPOSE 8080

# Command to run
CMD ["./main"]