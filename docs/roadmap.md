# Roadmap to v1.0.0

> Last updated: 2026-05-19

Detour is a single Go binary that routes Claude Code model requests between a local inference server and the Anthropic API. This roadmap tracks what needs to happen before we tag `v1.0.0`.

## Current state

The core proxy loop works: model-name routing, SSE streaming, thinking-block stripping, message-start patching, header filtering, request validation, graceful shutdown, and config persistence. CI runs `go vet` + `go test -race`, Docker builds are smoke-tested, and GoReleaser produces cross-platform binaries. No version has been tagged yet.

---

## v0.0.0 — Foundation (done)

- [x] Model-name routing (`--model-name` alias vs. passthrough)
- [x] SSE streaming proxy with `[DONE]` token handling
- [x] Thinking-block stripping and message-start patching
- [x] Header filtering (auth, beta headers)
- [x] Request size limiting and validation
- [x] Config persistence (`~/.detour/config.json`)
- [x] Cleartext upstream warning
- [x] Subprocess launcher with environment injection
- [x] Graceful shutdown on SIGINT
- [x] Mock LLM server for offline testing
- [x] CI pipeline (test + vet + Docker smoke)
- [x] GoReleaser config for cross-platform binaries
- [x] Docker image with multi-stage build
- [x] Release workflow (tag-triggered binary + GHCR image publish)

## v0.1.0 — Stability & correctness

- [ ] **Version injection**: Wire `-ldflags -X main.version` into `Makefile build` target so `detour --version` reports the real version (not `dev`)
- [x] **`--version` flag**: Print version and exit
- [x] **Upstream health pre-check**: On startup, attempt a lightweight request to the local model API and warn (not fail) if it's unreachable
- [ ] **Retry on transient local failures**: One automatic retry with backoff on 502/503/connection-refused from the local backend before returning an error
- [ ] **Request timeout flag**: Expose `--timeout` to cap how long the proxy waits for a response from either upstream (default: no timeout for streaming, 120s for non-streaming)
- [ ] **Error response normalization**: Ensure all proxy-generated errors follow the Anthropic error envelope (`{"type":"error","error":{...}}`) so Claude Code handles them gracefully

## v0.2.0 — Observability

- [ ] **Structured logging**: Replace `fmt.Fprintf(os.Stderr, ...)` with a lightweight structured logger (zerolog or slog); keep zero-dependency default, gate behind `DETOUR_LOG`
- [ ] **Per-request metrics to stderr**: When logging is on, emit model name, route decision, upstream latency, status code, and token counts (from response headers) per request
- [ ] **`--log-file` flag**: Optionally write logs to a file instead of stderr

## v0.3.0 — Configuration & UX

- [ ] **Environment variable parity**: Accept `DETOUR_MODEL_NAME`, `DETOUR_MODEL_API`, `DETOUR_PORT` everywhere (not just Docker) — document precedence: flags > env > saved config
- [ ] **`detour config show`**: Print the resolved config (merged flags/env/saved) as JSON for debugging
- [ ] **`detour config reset`**: Delete saved config file
- [ ] **Config file path override**: `--config` flag or `DETOUR_CONFIG` env var to use a non-default config directory

## v0.4.0 — Testing & CI

- [ ] **Integration test harness**: End-to-end tests that start the proxy + mock LLM, send requests through, and assert on routing, streaming, and error behavior
- [ ] **Coverage reporting**: Add `-coverprofile` to CI and publish a coverage badge
- [ ] **Golangci-lint in CI**: Add a lint step with a curated config
- [ ] **Fuzz testing**: Fuzz the request body parser and SSE chunker for malformed input resilience

## v1.0.0 — Documentation & release

- [ ] **Man page or `--help` improvements**: Expand help text with examples for common workflows (vLLM, llama.cpp, Docker)
- [ ] **CHANGELOG.md**: Start a changelog following Keep a Changelog format
- [ ] **CONTRIBUTING.md**: Contribution guide covering build, test, and release process
- [ ] **Architecture doc**: Diagram of the proxy pipeline (request → route → transform → forward → stream back)
- [ ] **Tag `v1.0.0`**: Semantic versioning starts here

---

## Deferred

These features have been considered but are blocked on external constraints:

- **Multiple model routes** (`--model-name red=http://host1,blue=http://host2`): Claude Code's UI has no mechanism for selecting between multiple local models within a session, so there is no way to coherently expose this. Revisit if Claude Code adds model-switching support.

## Non-goals for v1.0

These are explicitly out of scope to keep the release focused:

- **OpenAI-compatible backends**: v1 only speaks the Anthropic Messages API
- **TLS termination**: The proxy binds to localhost; TLS is the user's problem
- **Multi-user / auth**: Detour is a single-user local tool
- **Plugin or middleware system**: The transformation pipeline is hardcoded
- **GUI or web dashboard**: CLI-only

## How to contribute

Pick any unchecked item, open a PR, and reference this roadmap. Milestones are roughly ordered by priority but items within a milestone can land independently.
