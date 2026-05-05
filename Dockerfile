# Docker Container for Detour + Claude Code
# Multi-stage build: builder stage produces static detour binary
# runtime stage assembles the final minimal image

# Stage 1: Builder - build detour static binary
FROM golang:1.24-alpine AS builder

WORKDIR /src

# Install build tools
RUN apk add --no-cache git

# Copy go module files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build detour static binary
# CGO_ENABLED=0 for static binary
# -trimpath and -ldflags='-s -w' for smaller, reproducible builds
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /usr/local/bin/detour ./cmd/detour

# Stage 2: Runtime - minimal Alpine image with detour and Claude Code
FROM alpine:3.20

# Install runtime dependencies
# nodejs, npm: required for @anthropic-ai/claude-code
# bash: Claude Code Bash tool defaults to bash semantics
# git: Claude Code uses git for status, diff, commits
# curl: used by WebFetch and other tools
# ca-certificates: TLS connections to api.anthropic.com and HTTPS upstreams
# tini: PID 1 reaper so Ctrl-C kills both proxy and claude
RUN apk add --no-cache \
    nodejs \
    npm \
    bash \
    git \
    curl \
    ca-certificates \
    tini

# Install Claude Code CLI globally
# Pin to a specific version for reproducibility
RUN npm install -g @anthropic-ai/claude-code

# Copy static binary and entrypoint from builder stage
COPY --from=builder /usr/local/bin/detour /usr/local/bin/detour

# Copy entrypoint script (created in D2)
COPY scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]

# Default command runs claude
CMD ["claude"]
