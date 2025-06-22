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
    bash

# Update npm to latest version to ensure npx is available
RUN npm install -g npm@latest

# Install uv (fast Python package installer/manager)
RUN curl -LsSf https://astral.sh/uv/install.sh | sh
ENV PATH="/root/.cargo/bin:$PATH"

# Install common Python tools that MCP servers might need
# Use --break-system-packages for Docker container environment
RUN pip3 install --no-cache-dir --break-system-packages \
    httpx \
    aiohttp \
    requests \
    pydantic \
    sqlite-utils \
    click

# Install common Node.js global packages for MCP servers
RUN npm install -g \
    typescript \
    ts-node

# Create symlinks for common Python commands
RUN ln -sf /usr/bin/python3 /usr/bin/python

# Verify installations and create npx fallback if needed
RUN node --version && npm --version && python --version && uv --version
RUN npx --version || (echo '#!/bin/sh\nexec npm exec -- "$@"' > /usr/local/bin/npx && chmod +x /usr/local/bin/npx)
RUN npx --version

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/main .

# Create directories for config (config.json will be mounted at runtime)
RUN mkdir -p /app /config

# Expose port
EXPOSE 8080

# Command to run
CMD ["./main"]