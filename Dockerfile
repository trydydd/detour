# Docker Container for Detour + Claude Code
# Multi-stage build: builder stage produces static detour binary
# runtime stage assembles the final minimal image

# Stage 1: Builder - build detour static binary
FROM golang:1.24-alpine AS builder

WORKDIR /src

# Install build tools
RUN apk add --no-cache git

# Copy go module files first for better caching
# COPY go.mod go.sum ./
# RUN go mod download

# Copy source code (excluding non-essential files via .dockerignore)
COPY . .

# Build detour static binary
# CGO_ENABLED=0 for static binary
# -trimpath and -ldflags='-s -w' for smaller, reproducible builds
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /usr/local/bin/detour ./cmd/detour

# Stage 2: Runtime - minimal Alpine image with detour and Claude Code
FROM alpine:3.20

# Install runtime dependencies
# bash: Claude Code Bash tool defaults to bash semantics
# git: Claude Code uses git for status, diff, commits
# curl: used by WebFetch and other tools
# ca-certificates: TLS connections to api.anthropic.com and HTTPS upstreams
# tini: PID 1 reaper so Ctrl-C kills both proxy and claude
# libgcc, libstdc++: required for Claude Code on musl-based distributions
# ripgrep: required for search functionality on Alpine (USE_BUILTIN_RIPGREP=0)
RUN apk add --no-cache \
    bash \
    git \
    curl \
    ca-certificates \
    tini \
    libgcc \
    libstdc++ \
    ripgrep

# Install Claude Code via npm
RUN apk add --no-cache nodejs npm && \
    npm install -g @anthropic-ai/claude-code

# Set sandbox environment variable
ENV IS_SANDBOX=1

# Copy static binary and entrypoint from builder stage
COPY --from=builder /usr/local/bin/detour /usr/local/bin/detour

# Copy entrypoint script (created in D2)
COPY scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

# Set entrypoint — the script always invokes claude unless --no-claude is given
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
