<p align="center">
  <img src=".github/detour.svg" alt="DETOUR" width="600">
</p>

# detour

A single Go binary that routes Claude Code's model requests between a local inference server and the real Anthropic API.

```
detour --model-name red --model-api http://192.168.0.28:8000
```

That's it. Detour starts a local proxy, injects the right env vars, and launches Claude Code as a subprocess. Your existing `~/.claude/settings.json` is never touched.

## How it works

Claude Code sends all API requests to detour instead of `api.anthropic.com`. Detour inspects the `model` field:

- **`model: "red"`** (or whatever alias you chose) → forwarded to your local inference server
- **`model` contains `"opus"`** → forwarded unchanged to `api.anthropic.com`

This means `claude --model opusplan` works correctly: planning calls go to Opus on Anthropic, execution calls go to your local model.

## Requirements

- Go 1.22+ (to build) or grab the binary from releases
- A local inference server that speaks the **Anthropic Messages API natively**:
  - **vLLM**: `vllm serve Qwen/Qwen2.5-Coder-7B-Instruct --served-model-name red --enable-auto-tool-choice --tool-call-parser hermes`
  - **llama.cpp server**: `llama-server --model model.gguf --alias red`
- A real Anthropic API key (used for Opus passthrough)

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

### All flags

| Flag | Default | Description |
|---|---|---|
| `--model-name` | — | Alias Claude Code uses as the model name (required) |
| `--model-api` | — | Base URL of local inference server (required) |
| `--port` | `8888` | Local proxy port |
| `--no-haiku` | false | Don't override the haiku model tier |

### Proxy-only mode (no subprocess)

If you want to manage Claude yourself:

```bash
detour --model-name red --model-api http://192.168.0.28 -- --help
# or just set ANTHROPIC_BASE_URL manually and run claude
```

## Verify routing

Check detour's stderr output — it logs which backend each request goes to:

```
detour: proxy on :8888  [red → local | opus → anthropic]
```
