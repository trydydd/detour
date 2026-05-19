# Docker Image Guide

This guide covers using the detour container image with Claude Code.

## Image Variants and Tags

| Tag | Description |
|-----|-------------|
| `latest` | Most recent release |
| `vX.Y.Z` | Specific version tag |
| `edge` | Latest commit on main branch |
| `sha-<short>` | Specific commit on main |

**Architecture:** `linux/amd64` and `linux/arm64` (multi-arch manifest).

---

## Build It Yourself

```bash
make image
```

This builds `detour:dev` locally.

To clean up:
```bash
make image-clean
```

---

## Auth Modes

### 1. Anthropic API Key
```bash
docker run -it --rm \
  -e ANTHROPIC_API_KEY=your-key-here \
  -e DETOUR_MODEL_NAME=red \
  -e DETOUR_MODEL_API=http://host.docker.internal:8080 \
  ghcr.io/trydydd/detour
```

### 2. Persistent OAuth (Volume Mount)
```bash
docker run -it --rm \
  -v claude-config:/root/.claude \
  -e DETOUR_MODEL_NAME=red \
  -e DETOUR_MODEL_API=http://host.docker.internal:8080 \
  ghcr.io/trydydd/detour
```

First run: `claude login` inside the container to save tokens.

### 3. Ephemeral OAuth (Interactive)
```bash
docker run -it ghcr.io/trydydd/detour
# Then run claude login inside
```

### 4. No Auth
```bash
docker run -it ghcr.io/trydydd/detour
# Claude prompts for login on first request
```

---

## Volume Strategy

### Mount Project Directory
```bash
docker run -it --rm \
  -v "$(pwd)":/work \
  -w /work \
  -e DETOUR_MODEL_NAME=red \
  ghcr.io/trydydd/detour
```

Claude Code will see your project at `/work`.

### Claude Config Volume
For persistent OAuth tokens across container restarts:
```bash
docker volume create claude-config
docker run -it --rm -v claude-config:/root/.claude ghcr.io/trydydd/detour
```

---

## Networking to a Host-Side LLM

### Linux (host network)
```bash
docker run -it --rm \
  --network=host \
  -e DETOUR_MODEL_NAME=red \
  -e DETOUR_MODEL_API=http://127.0.0.1:8080 \
  ghcr.io/trydydd/detour
```

### macOS / Windows (Docker Desktop)
```bash
docker run -it --rm \
  -e DETOUR_MODEL_NAME=red \
  -e DETOUR_MODEL_API=http://host.docker.internal:8080 \
  ghcr.io/trydydd/detour
```

---

## Passing Claude Flags

Use `--` after the image name to pass flags to `claude`:

```bash
# Skip permissions check
docker run -it --rm ghcr.io/trydydd/detour -- --dangerously-skip-permissions

# Run with verbose logging
docker run -it --rm ghcr.io/trydydd/detour -- claude --verbose

# Empty args (just run claude)
docker run -it --rm ghcr.io/trydydd/detour --
```

---

## Cleartext Upstream Warning

Detour routes traffic to your local inference server over HTTP (not HTTPS). This is a security risk if:

1. Your LLM server accepts requests from any network interface
2. You're on an untrusted network

**Mitigation:** Use SSH tunneling or ensure your LLM server binds only to `127.0.0.1`.

See [AUDIT.md](../AUDIT.md) and [H4](../AUDIT.md#h4) for security details.

---

## Troubleshooting

### Claude exits with code 1
Check container logs:
```bash
docker run --rm ghcr.io/trydydd/detour --no-claude --help
```

Verify the model API is reachable from within the container.

### Port already in use
Detour binds to port 8888 by default. Change it:
```bash
-e DETOUR_PORT=9999
```

### Token mismatch errors
Ensure `ANTHROPIC_DETOUR_AUTH` matches between detour and Claude Code. The container generates a random token if unset.

### Image size too large
The target is ≤350 MB. If your build exceeds this:
- Check for unused packages in the Dockerfile
- Consider using `--no-cache` for builds

---

## Running Rootless

The default image runs as root (UID 0). For non-root operation:

```bash
# Method 1: Use --user flag
docker run -it --rm --user 1000:1000 ghcr.io/trydydd/detour

# Method 2: Build with RUNTIME_USER=root to revert to root
docker build --build-arg RUNTIME_USER=root -t detour:root .
```

When mounting volumes as non-root, ensure proper ownership:
```bash
sudo chown -R 1000:1000 /path/to/mount
```
