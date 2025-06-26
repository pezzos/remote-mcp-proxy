# Build stage
FROM golang:1.21-alpine3.18 AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source code
COPY . .

# Build the application
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o main .

# Final stage
FROM alpine:3.18

# Install base dependencies for various MCP server types
RUN apk --no-cache add \
    ca-certificates \
    nodejs npm \
    python3 py3-pip \
    curl wget \
    git \
    bash \
    sqlite \
    jq \
 && npm install -g npm@latest \
 && curl -LsSf https://astral.sh/uv/install.sh | sh \
 && ln -sf /root/.local/bin/uv /usr/local/bin/uv 2>/dev/null || true \
 && pip3 install --no-cache-dir --break-system-packages \
        requests \
        httpx \
        pydantic \
 && npm install -g typescript \
 && ln -sf /usr/bin/python3 /usr/bin/python \
 && node --version && npm --version && python --version \
 && npx --help >/dev/null 2>&1 || (echo '#!/bin/sh\nexec npm exec -- "$@"' > /usr/local/bin/npx && chmod +x /usr/local/bin/npx) \
 && npx --version \
 && echo "Pre-installing MCP packages..." \
 && npm install -g @modelcontextprotocol/server-filesystem \
 && npm install -g @modelcontextprotocol/server-memory \
 && npm install -g @modelcontextprotocol/server-sequential-thinking \
 && npm install -g @notionhq/notion-mcp-server \
 && echo "Build completed successfully" \
 && mkdir -p /app /config

ENV PATH="/root/.local/bin:/root/.cargo/bin:$PATH"

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/main .

# Copy configuration file and scripts
COPY config.json /app/config.json
COPY scripts/ /app/scripts/

# Expose port
EXPOSE 8080

# Health check to mirror docker-compose
HEALTHCHECK --interval=30s --timeout=10s --retries=3 CMD curl -f http://localhost:8080/health || exit 1

# Command to run
CMD ["/app/scripts/startup.sh"]