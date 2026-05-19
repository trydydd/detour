# detour

[![CI](https://github.com/trydydd/detour/actions/workflows/ci.yml/badge.svg)](https://github.com/trydydd/detour/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/trydydd/detour?sort=semver)](https://github.com/trydydd/detour/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/trydydd/detour)](go.mod)

A single Go binary that routes Claude Code's model requests between a local inference server and the real Anthropic API.

```
detour --model-name red --model-api http://192.168.0.28
```

That's it. Detour starts a local proxy, injects the right env vars, and launches Claude Code as a subprocess. Your existing `~/.claude/settings.json` is never touched.

## How it works

Claude Code sends all API requests to detour instead of `api.anthropic.com`. Detour inspects the `model` field on each `/v1/messages` call:

- **Model name matches your `--model-name` alias exactly** → forwarded to your local inference server
- **Anything else** (including `claude-opus-*`, `claude-sonnet-*`, `claude-haiku-*`) → forwarded unchanged to `api.anthropic.com`

This means modes that switch models per-turn — like `claude --model opusplan` — work correctly: planning turns ask for Opus and pass through to Anthropic, execution turns ask for your alias and route to the local model.

## Requirements

- Go 1.24+ (to build) or grab the binary from [releases](https://github.com/trydydd/detour/releases)
- A local inference server that speaks the **Anthropic Messages API natively**:
  - **vLLM**: `vllm serve Qwen/Qwen2.5-Coder-7B-Instruct --served-model-name red --enable-auto-tool-choice --tool-call-parser hermes`
  - **llama.cpp server**: `llama-server --model model.gguf --alias red`
- A real Anthropic API key (used for passthrough turns)

> The `--served-model-name` (vLLM) or `--alias` (llama.cpp) **must match** your `--model-name` flag.

## Install

```bash
go install github.com/trydydd/detour/cmd/detour@latest
```

## Usage

```bash
detour --model-name red --model-api http://192.168.0.28 [-- claude args]
```

Flags are saved to `~/.detour/config.json` on first run. Subsequent invocations need no flags:

```bash
detour   # uses saved config
```

## Run with Docker

The official container image is available at `ghcr.io/trydydd/detour`.


**Persistent OAuth via volume:**
```bash
docker run -it --rm \
  -v "$HOME/.claude:/home/detour/.claude" \
  -e DETOUR_MODEL_NAME=red \
  -e DETOUR_MODEL_API=http://host.docker.internal:8080 \
  ghcr.io/trydydd/detour
```

**Passing claude flags (e.g., skip permissions):**
```bash
docker run -it --rm \
  -e DETOUR_MODEL_NAME=red \
  -e DETOUR_MODEL_API=http://host.docker.internal:8080 \
  ghcr.io/trydydd/detour -- --dangerously-skip-permissions
```

Environment variables `DETOUR_MODEL_NAME`, `DETOUR_MODEL_API`, and `DETOUR_PORT` configure the detour proxy. See [docs/docker.md](docs/docker.md) for full documentation including auth modes, volume strategy, networking, and troubleshooting.

**Using a local Docker image:**

Run with a specific model and inference server:
```bash
docker run --rm -it \
  -u $(id -u):$(id -g) \
  -v "$(pwd):/workspace" \
  -v "$HOME/.config/claude-container:/claude" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e "CLAUDE_CONFIG_DIR=/claude" \
  detour:latest --model-name red --model-api http://192.168.0.214:8000 -- --model Qwen/Qwen3.6-35B-A3B-FP8
```

With a prompt:
```bash
docker run --rm -it \
  -u $(id -u):$(id -g) \
  -v "$(pwd):/workspace" \
  -v "$HOME/.config/claude-container:/claude" \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e "CLAUDE_CONFIG_DIR=/claude" \
  detour:latest --model-name red --model-api http://192.168.0.214:8000 -- --model red -p "what is the capital of france"
```

This configuration:
- Mounts the current directory for file access
- Persists Claude configuration in `~/.config/claude-container`
- Exposes the Docker socket for container-in-container support
- Sets `CLAUDE_CONFIG_DIR` to use the mounted config path

### Flags

| Flag | Default | Description |
|---|---|---|
| `--model-name` | — | Alias Claude Code uses as the model name (required) |
| `--model-api` | — | Base URL of local inference server (required) |
| `--port` | `8888` | Local proxy port (bound to `127.0.0.1`) |

### Environment variables

| Variable | Description |
|---|---|
| `DETOUR_LOG` | Set to `1`/`true`/`yes`/`on` to log one line per proxied request to stderr (`detour: <method> <path> <status> <duration>`). Off by default. |

### Proxy-only mode (no subprocess)

If you want to manage Claude yourself:

```bash
detour --model-name red --model-api http://192.168.0.28 -- --help
# or just set ANTHROPIC_BASE_URL manually and run claude
```

## Verify routing

On startup, detour prints the bound address and which alias it routes locally:

```
detour: proxy on 127.0.0.1:8888  [red → local | * → anthropic]
```

For per-request visibility, run with `DETOUR_LOG=1`.

## Testing with the mock LLM

`cmd/mockllm` is a dependency-free server that speaks the Anthropic Messages API (streaming and non-streaming) and always replies with `"THIS IS DETOUR TEST!"`. It's the fastest way to verify routing without standing up real local inference.

```bash
# Terminal 1: start the mock on port 9999
go run ./cmd/mockllm --port 9999

# Terminal 2: point detour at the mock and launch Claude Code
~/go/bin/detour --model-name detour-mock --model-api http://127.0.0.1:9999
```

Any prompt routed to the local backend will come back as `THIS IS DETOUR TEST!`, confirming the request reached the mock through the proxy. Prompts routed to Anthropic models still hit the real API as usual.
