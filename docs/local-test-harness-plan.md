# Local Test Harness Implementation Plan

## Overview

This plan describes how to implement a self-contained test harness that:

1. Spins up a real Ollama model as the "local" inference backend
2. Starts a sentinel server (fake Anthropic) to detect leaked traffic
3. Starts the detour proxy wired to both backends
4. Runs two verification tests and prints a clear pass/fail report

The user can run this harness to know with certainty that traffic routed to
their local model is NOT hitting the real Anthropic API, and that responses
actually originate from their local LLM.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Harness Process                         │
│                                                              │
│  ┌─────────────────┐                                         │
│  │   Test runner   │─── POST /v1/messages ─────────────────►│
│  │   (main.go)     │    X-Detour-Auth: <token>               │
│  └─────────────────┘                                         │
│          ▲                                                    │
│          │                                                    │
│          │         ┌──────────────────────────────────────┐  │
│          │         │       Detour Proxy (:18888)          │  │
│          │         │                                      │  │
│          │         │  model == local alias                │  │
│          └─────────┤      → LocalUpstreamURL ─────────────┼─►Adapter
│                    │                                      │      │
│                    │  model == anything else              │      ▼ POST /api/chat
│                    │      → AnthropicUpstreamURL ─────────┼─►Sentinel
│                    │                                      │  (fake Anthropic)
│                    └──────────────────────────────────────┘      │
│                                                              returns SENTINEL_MARKER
└─────────────────────────────────────────────────────────────┘
                                  │ Adapter translates:
                                  │ Anthropic /v1/messages → Ollama /api/chat
                                  ▼
                         Ollama (:11434) — real model
```

**Why an adapter?** The detour proxy forwards requests in Anthropic Messages API
format (`POST /v1/messages` with `{"model":..., "messages":[...], "max_tokens":...}`).
Ollama does not speak the Anthropic API natively — it uses its own `/api/chat`
format. The adapter bridges this gap transparently.

---

## Files to Create

```
detour/
├── cmd/
│   └── harness/
│       └── main.go              ← harness binary entry point (~200 lines)
├── internal/
│   └── harness/
│       ├── sentinel.go          ← fake Anthropic server (~70 lines)
│       └── adapter.go           ← Anthropic→Ollama translation (~100 lines)
└── scripts/
    └── harness-setup.sh         ← convenience script to pull model (~25 lines)
```

**No existing files are modified.**

---

## Module and Go Version

- Module path: `github.com/trydydd/detour`
- Go version: `1.24`
- No new external dependencies — only standard library + existing internal packages

---

## Authentication Details

The detour proxy reads `ANTHROPIC_DETOUR_AUTH` from the environment **at proxy
construction time** (when `proxy.NewMux()` is called) and requires every request
to include `X-Detour-Auth: <token>` header matching that value.

The harness must:
1. Generate a random hex token
2. Call `os.Setenv("ANTHROPIC_DETOUR_AUTH", token)` **before** `proxy.NewMux()`
3. Include `X-Detour-Auth: <token>` in every test HTTP request

---

## File 1: `internal/harness/sentinel.go`

**Package:** `harness`

### Purpose

A fake Anthropic API server that returns a unique sentinel marker string in every
response and records how many times it has been hit. If a request intended for
the local model ever reaches this server, the sentinel marker will appear in the
response — proving that traffic leaked from the local backend to the Anthropic
backend.

### Sentinel Marker Constant

```
DETOUR_HARNESS_SENTINEL_TRAFFIC_LEAK_DETECTED
```

This string is embedded in every response body from the sentinel. It cannot
appear in any Ollama response because: (a) no test prompt requests it, and (b)
it is not a natural language phrase any LLM would spontaneously generate.

### Complete File

```go
package harness

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
)

// SentinelMarker is embedded in every sentinel response.
// Its presence in a local-model response proves traffic leaked to the Anthropic backend.
const SentinelMarker = "DETOUR_HARNESS_SENTINEL_TRAFFIC_LEAK_DETECTED"

// Sentinel is a fake Anthropic API server that records how many times it is hit.
type Sentinel struct {
	server   *httptest.Server
	hitCount atomic.Int64
}

// NewSentinel starts a new sentinel server and returns it. It is already listening.
func NewSentinel() *Sentinel {
	s := &Sentinel{}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/messages", s.handleMessages)
	mux.HandleFunc("/v1/models", s.handleModels)
	mux.HandleFunc("/harness/status", s.handleStatus)
	s.server = httptest.NewServer(mux)
	return s
}

// URL returns the base URL of the sentinel server (e.g., "http://127.0.0.1:12345").
func (s *Sentinel) URL() string { return s.server.URL }

// HitCount returns the number of times the sentinel has been called.
func (s *Sentinel) HitCount() int64 { return s.hitCount.Load() }

// Close shuts down the sentinel server.
func (s *Sentinel) Close() { s.server.Close() }

func (s *Sentinel) handleMessages(w http.ResponseWriter, r *http.Request) {
	s.hitCount.Add(1)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":            "msg_sentinel_001",
		"type":          "message",
		"role":          "assistant",
		"content":       []map[string]any{{"type": "text", "text": SentinelMarker}},
		"model":         "sentinel",
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage":         map[string]int{"input_tokens": 1, "output_tokens": 1},
	})
}

func (s *Sentinel) handleModels(w http.ResponseWriter, r *http.Request) {
	s.hitCount.Add(1)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"data": []map[string]any{
			{"id": "sentinel-model", "display_name": SentinelMarker, "type": "model"},
		},
	})
}

func (s *Sentinel) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{"hit_count": s.hitCount.Load()})
}
```

---

## File 2: `internal/harness/adapter.go`

**Package:** `harness`

### Purpose

The detour proxy sends `POST /v1/messages` requests in Anthropic format to
`LocalUpstreamURL`. Ollama does not speak this format — it uses `POST /api/chat`.
The adapter translates between the two formats.

**Request translation (Anthropic → Ollama):**

| Anthropic field | Ollama field | Notes |
|---|---|---|
| `model` | `model` | Adapter uses its own model name |
| `messages[].role` | `messages[].role` | Direct copy |
| `messages[].content` (string) | `messages[].content` | Direct copy |
| `max_tokens` | (dropped) | Ollama ignores |
| (none) | `stream` | Set to `false` always |

**Response translation (Ollama → Anthropic):**

| Ollama field | Anthropic field | Notes |
|---|---|---|
| `message.content` | `content[0].text` | Wrapped in text block array |
| `model` | `model` | Direct copy |
| (none) | `id` | Hard-coded `"msg_adapter_001"` |
| (none) | `type` | `"message"` |
| (none) | `role` | `"assistant"` |
| (none) | `stop_reason` | `"end_turn"` |

### Data Structures

**Incoming Anthropic request:**
```go
type anthropicRequest struct {
    Model     string             `json:"model"`
    Messages  []anthropicMessage `json:"messages"`
    MaxTokens int                `json:"max_tokens"`
}

type anthropicMessage struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}
```

**Outgoing Ollama request:**
```go
type ollamaRequest struct {
    Model    string          `json:"model"`
    Messages []ollamaMessage `json:"messages"`
    Stream   bool            `json:"stream"`
}

type ollamaMessage struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}
```

**Incoming Ollama response:**
```go
type ollamaResponse struct {
    Model   string        `json:"model"`
    Message ollamaMessage `json:"message"`
    Done    bool          `json:"done"`
}
```

### Complete File

```go
package harness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
)

type anthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []anthropicMessage `json:"messages"`
	MaxTokens int                `json:"max_tokens"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaResponse struct {
	Model   string        `json:"model"`
	Message ollamaMessage `json:"message"`
	Done    bool          `json:"done"`
}

// Adapter translates Anthropic Messages API requests to Ollama /api/chat format.
type Adapter struct {
	server    *httptest.Server
	ollamaURL string
	model     string
}

// NewAdapter starts an adapter server that forwards translated requests to the
// given Ollama instance using the given model. The server is already listening.
func NewAdapter(ollamaURL, model string) *Adapter {
	a := &Adapter{ollamaURL: ollamaURL, model: model}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/messages", a.handleMessages)
	a.server = httptest.NewServer(mux)
	return a
}

// URL returns the adapter's base URL.
func (a *Adapter) URL() string { return a.server.URL }

// Close shuts down the adapter server.
func (a *Adapter) Close() { a.server.Close() }

func (a *Adapter) handleMessages(w http.ResponseWriter, r *http.Request) {
	// 1. Read and decode the incoming Anthropic request.
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeAdapterError(w, http.StatusBadRequest, "could not read request body")
		return
	}
	var req anthropicRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		writeAdapterError(w, http.StatusBadRequest, "could not parse request JSON: "+err.Error())
		return
	}

	// 2. Build the Ollama request. Use a.model (not req.Model) because req.Model
	//    is the local alias detour assigned; Ollama needs the real model tag.
	msgs := make([]ollamaMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = ollamaMessage{Role: m.Role, Content: m.Content}
	}
	ollamaReq := ollamaRequest{Model: a.model, Messages: msgs, Stream: false}
	ollamaBody, err := json.Marshal(ollamaReq)
	if err != nil {
		writeAdapterError(w, http.StatusInternalServerError, "could not marshal ollama request: "+err.Error())
		return
	}

	// 3. Send to Ollama.
	resp, err := http.Post(a.ollamaURL+"/api/chat", "application/json", bytes.NewReader(ollamaBody))
	if err != nil {
		writeAdapterError(w, http.StatusBadGateway, "ollama unavailable: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		writeAdapterError(w, http.StatusBadGateway,
			fmt.Sprintf("ollama returned HTTP %d: %s", resp.StatusCode, string(respBody)))
		return
	}

	// 4. Decode Ollama response.
	var ollamaResp ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		writeAdapterError(w, http.StatusBadGateway, "could not decode ollama response: "+err.Error())
		return
	}

	// 5. Translate to Anthropic format and respond.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"id":            "msg_adapter_001",
		"type":          "message",
		"role":          "assistant",
		"content":       []map[string]any{{"type": "text", "text": ollamaResp.Message.Content}},
		"model":         a.model,
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage":         map[string]int{"input_tokens": 1, "output_tokens": 1},
	})
}

func writeAdapterError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"type":  "error",
		"error": map[string]string{"type": "proxy_error", "message": msg},
	})
}
```

---

## File 3: `cmd/harness/main.go`

**Package:** `main`

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--model-name` | string | `qwen2:0.5b` | Ollama model tag to test with |
| `--ollama-url` | string | `http://localhost:11434` | Base URL of Ollama server |
| `--port` | int | `18888` | Port for the detour proxy |

### Startup Sequence (order is critical)

1. Parse flags.
2. Call `checkOllama(ollamaURL, modelName)` — exits with error if Ollama unreachable or model not pulled.
3. Start adapter: `adapter := harness.NewAdapter(ollamaURL, modelName)`, defer `adapter.Close()`.
4. Start sentinel: `sentinel := harness.NewSentinel()`, defer `sentinel.Close()`.
5. Generate auth token (32 random bytes, hex-encoded).
6. **`os.Setenv("ANTHROPIC_DETOUR_AUTH", authToken)`** ← MUST happen before step 7.
7. Build `proxy.Config`:
   - `ModelName` = `modelName` (the local alias detour uses for routing)
   - `LocalUpstreamURL` = `adapter.URL()` (NOT Ollama directly; adapter translates)
   - `AnthropicUpstreamURL` = `sentinel.URL()` (NOT real Anthropic; sentinel records hits)
8. Call `proxy.NewMux(proxyCfg)`.
9. Start `http.Server` in a goroutine.
10. Call `waitForPort(addr, 3s)` — retry TCP dial until proxy is ready.
11. Run test suite.
12. Gracefully shutdown proxy with a 2s context timeout.
13. Print final summary; `os.Exit(1)` if any test failed.

### Complete File

```go
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/trydydd/detour/internal/harness"
	"github.com/trydydd/detour/internal/proxy"
)

func main() {
	modelName := flag.String("model-name", "qwen2:0.5b", "Ollama model name (e.g., qwen2:0.5b, tinyllama)")
	ollamaURL := flag.String("ollama-url", "http://localhost:11434", "Base URL of Ollama server")
	port := flag.Int("port", 18888, "Port for detour proxy to listen on")
	flag.Parse()

	fmt.Println("detour local test harness")
	fmt.Println("═══════════════════════════════════════")
	fmt.Printf("  local model:  %s\n", *modelName)
	fmt.Printf("  ollama:       %s\n", *ollamaURL)
	fmt.Printf("  proxy port:   %d\n\n", *port)

	// Pre-flight: verify Ollama is up and model is present.
	fmt.Println("checking ollama...")
	checkOllama(*ollamaURL, *modelName)
	fmt.Printf("  ✓ ollama reachable, model %q is available\n\n", *modelName)

	// Start adapter (Anthropic → Ollama translation layer).
	adapter := harness.NewAdapter(*ollamaURL, *modelName)
	defer adapter.Close()
	fmt.Printf("adapter started:  %s\n", adapter.URL())

	// Start sentinel (fake Anthropic).
	sentinel := harness.NewSentinel()
	defer sentinel.Close()
	fmt.Printf("sentinel started: %s\n", sentinel.URL())

	// Generate auth token and export it BEFORE building the proxy mux.
	// proxy.NewMux captures os.Getenv("ANTHROPIC_DETOUR_AUTH") at construction time.
	authToken := generateToken()
	os.Setenv("ANTHROPIC_DETOUR_AUTH", authToken)

	// Build the detour proxy wired to the adapter and sentinel.
	proxyCfg := &proxy.Config{
		ModelName:            *modelName,
		LocalUpstreamURL:     adapter.URL(),
		AnthropicUpstreamURL: sentinel.URL(),
	}
	mux := proxy.NewMux(proxyCfg)
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, "harness: proxy error:", err)
		}
	}()
	if err := waitForPort(addr, 3*time.Second); err != nil {
		fmt.Fprintln(os.Stderr, "harness: proxy did not start:", err)
		os.Exit(1)
	}
	proxyBase := "http://" + addr
	fmt.Printf("proxy started:    %s\n\n", proxyBase)

	// Run verification tests.
	passed := 0
	failed := 0

	fmt.Println("═══════════════════════════════════════")
	fmt.Println("TEST 1: local model alias routes to Ollama (not Anthropic)")
	if testLocalModelStaysLocal(proxyBase, authToken, *modelName, sentinel) {
		passed++
	} else {
		failed++
	}

	fmt.Println("\nTEST 2: passthrough model routes to Anthropic backend")
	if testPassthroughHitsSentinel(proxyBase, authToken, sentinel) {
		passed++
	} else {
		failed++
	}

	// Graceful shutdown.
	shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)

	// Final summary.
	fmt.Println("\n═══════════════════════════════════════")
	if failed == 0 {
		fmt.Printf("RESULT: ALL TESTS PASSED (%d/%d)\n\n", passed, passed+failed)
		fmt.Println("Verified:")
		fmt.Printf("  • Traffic to %q stays local — Ollama, not Anthropic\n", *modelName)
		fmt.Println("  • Other model names route to the Anthropic backend (sentinel)")
		fmt.Println("  • Local model responses contain no sentinel marker")
	} else {
		fmt.Printf("RESULT: %d TEST(S) FAILED (%d/%d passed)\n", failed, passed, passed+failed)
		os.Exit(1)
	}
}

// testLocalModelStaysLocal sends a request using the local model alias and verifies:
//   - The sentinel was NOT hit (sentinel.HitCount() did not increase)
//   - The response does NOT contain harness.SentinelMarker
func testLocalModelStaysLocal(proxyBase, authToken, modelName string, sentinel *harness.Sentinel) bool {
	hitsBefore := sentinel.HitCount()
	body := fmt.Sprintf(
		`{"model":%q,"messages":[{"role":"user","content":"Reply with exactly the word: pong"}],"max_tokens":20}`,
		modelName,
	)
	respBody, statusCode, err := postMessages(proxyBase, authToken, body)
	if err != nil {
		fmt.Printf("  FAIL: HTTP error: %v\n", err)
		return false
	}
	if statusCode != http.StatusOK {
		fmt.Printf("  FAIL: proxy returned HTTP %d\n  body: %s\n", statusCode, respBody)
		return false
	}

	hitsAfter := sentinel.HitCount()
	if hitsAfter > hitsBefore {
		fmt.Printf("  FAIL: sentinel was hit %d time(s) — local model traffic leaked to Anthropic URL\n",
			hitsAfter-hitsBefore)
		return false
	}
	if strings.Contains(respBody, harness.SentinelMarker) {
		fmt.Printf("  FAIL: response contains sentinel marker — wrong backend served the request\n")
		fmt.Printf("  response: %s\n", firstN(respBody, 200))
		return false
	}

	text := extractText(respBody)
	fmt.Printf("  PASS: sentinel not hit (0 new hits), Ollama responded\n")
	fmt.Printf("        text: %q\n", firstN(text, 120))
	return true
}

// testPassthroughHitsSentinel sends a request using a non-local model name and verifies:
//   - The sentinel WAS hit (sentinel.HitCount() increased)
//   - The response contains harness.SentinelMarker
func testPassthroughHitsSentinel(proxyBase, authToken string, sentinel *harness.Sentinel) bool {
	hitsBefore := sentinel.HitCount()
	body := `{"model":"claude-3-5-sonnet-20241022","messages":[{"role":"user","content":"ping"}],"max_tokens":10}`
	respBody, statusCode, err := postMessages(proxyBase, authToken, body)
	if err != nil {
		fmt.Printf("  FAIL: HTTP error: %v\n", err)
		return false
	}
	// Sentinel returns 200; any other status is unexpected.
	if statusCode != http.StatusOK {
		fmt.Printf("  FAIL: unexpected HTTP %d\n  body: %s\n", statusCode, respBody)
		return false
	}

	hitsAfter := sentinel.HitCount()
	if hitsAfter <= hitsBefore {
		fmt.Printf("  FAIL: sentinel was NOT hit — passthrough routing is broken\n")
		return false
	}
	if !strings.Contains(respBody, harness.SentinelMarker) {
		fmt.Printf("  FAIL: response does not contain sentinel marker\n")
		fmt.Printf("  response: %s\n", firstN(respBody, 200))
		return false
	}

	fmt.Printf("  PASS: sentinel hit (%d new hit), response contains sentinel marker\n",
		hitsAfter-hitsBefore)
	return true
}

// postMessages sends a POST /v1/messages request to the proxy with the given JSON body.
// Returns (response body as string, HTTP status code, error).
func postMessages(proxyBase, authToken, jsonBody string) (string, int, error) {
	req, err := http.NewRequest(http.MethodPost, proxyBase+"/v1/messages", strings.NewReader(jsonBody))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Detour-Auth", authToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return string(body), resp.StatusCode, err
}

// extractText parses an Anthropic Messages API response and returns the text
// from the first text content block. Falls back to returning the raw body.
func extractText(responseBody string) string {
	var msg struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(responseBody), &msg); err != nil {
		return responseBody
	}
	for _, block := range msg.Content {
		if block.Type == "text" {
			return block.Text
		}
	}
	return responseBody
}

// checkOllama verifies Ollama is reachable and the target model is available.
// Exits with a helpful error message if either check fails.
func checkOllama(ollamaURL, modelName string) {
	resp, err := http.Get(ollamaURL + "/api/tags")
	if err != nil {
		fmt.Fprintf(os.Stderr, "harness: ollama not reachable at %s: %v\n", ollamaURL, err)
		fmt.Fprintf(os.Stderr, "harness: start ollama with:  ollama serve\n")
		os.Exit(1)
	}
	defer resp.Body.Close()

	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		fmt.Fprintf(os.Stderr, "harness: could not decode /api/tags response: %v\n", err)
		os.Exit(1)
	}

	for _, m := range tags.Models {
		if m.Name == modelName || strings.HasPrefix(m.Name, modelName+":") {
			return
		}
	}
	fmt.Fprintf(os.Stderr, "harness: model %q not found in ollama\n", modelName)
	fmt.Fprintf(os.Stderr, "harness: pull it with:        ollama pull %s\n", modelName)
	os.Exit(1)
}

// waitForPort retries a TCP dial to addr until it succeeds or the timeout expires.
func waitForPort(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", addr)
}

// generateToken returns a random 64-character hex string.
func generateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		fmt.Fprintln(os.Stderr, "harness: could not generate token:", err)
		os.Exit(1)
	}
	return hex.EncodeToString(b)
}

// firstN returns up to n characters of s.
func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
```

---

## File 4: `scripts/harness-setup.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail

MODEL="${1:-qwen2:0.5b}"

echo "Setting up detour test harness..."
echo "  model: $MODEL"
echo ""

# Verify ollama binary is present.
if ! command -v ollama &>/dev/null; then
    echo "ERROR: ollama is not installed."
    echo "       Install from: https://ollama.com/download"
    exit 1
fi

# Start ollama if it is not already running.
if ! curl -sf http://localhost:11434/api/tags >/dev/null 2>&1; then
    echo "Ollama is not running. Starting..."
    ollama serve &
    sleep 3
fi

# Pull model if not already present.
if ollama list | grep -q "^${MODEL}"; then
    echo "  ✓ $MODEL already available"
else
    echo "Pulling $MODEL (this may take a few minutes)..."
    ollama pull "$MODEL"
fi

echo ""
echo "Setup complete. Run the harness:"
echo "  go run ./cmd/harness --model-name $MODEL"
```

---

## Build and Run Instructions

```bash
# Step 1: Set up Ollama and pull the model (run once)
bash scripts/harness-setup.sh qwen2:0.5b

# Step 2: Build the harness binary
go build ./cmd/harness

# Step 3: Run the harness
./harness --model-name qwen2:0.5b

# Or run directly without building
go run ./cmd/harness --model-name qwen2:0.5b

# Custom flags example
go run ./cmd/harness \
  --model-name tinyllama \
  --ollama-url http://localhost:11434 \
  --port 18888
```

---

## Expected Output (all tests pass)

```
detour local test harness
═══════════════════════════════════════
  local model:  qwen2:0.5b
  ollama:       http://localhost:11434
  proxy port:   18888

checking ollama...
  ✓ ollama reachable, model "qwen2:0.5b" is available

adapter started:  http://127.0.0.1:XXXXX
sentinel started: http://127.0.0.1:YYYYY
proxy started:    http://127.0.0.1:18888

═══════════════════════════════════════
TEST 1: local model alias routes to Ollama (not Anthropic)
  PASS: sentinel not hit (0 new hits), Ollama responded
        text: "pong"

TEST 2: passthrough model routes to Anthropic backend
  PASS: sentinel hit (1 new hit), response contains sentinel marker

═══════════════════════════════════════
RESULT: ALL TESTS PASSED (2/2)

Verified:
  • Traffic to "qwen2:0.5b" stays local — Ollama, not Anthropic
  • Other model names route to the Anthropic backend (sentinel)
  • Local model responses contain no sentinel marker
```

---

## Failure Output Examples

**Test 1 failure — traffic leaked to Anthropic URL:**
```
TEST 1: local model alias routes to Ollama (not Anthropic)
  FAIL: sentinel was hit 1 time(s) — local model traffic leaked to Anthropic URL
```

**Test 1 failure — sentinel marker in local response:**
```
TEST 1: local model alias routes to Ollama (not Anthropic)
  FAIL: response contains sentinel marker — wrong backend served the request
  response: {"id":"msg_sentinel_001","type":"message",...,"text":"DETOUR_HARNESS_SENTINEL_TRAFFIC_LEAK_DETECTED"...
```

**Test 1 failure — Ollama not responding:**
```
TEST 1: local model alias routes to Ollama (not Anthropic)
  FAIL: proxy returned HTTP 502
  body: {"type":"error","error":{"type":"proxy_error","message":"ollama unavailable: ...
```

**Test 2 failure — passthrough not routing:**
```
TEST 2: passthrough model routes to Anthropic backend
  FAIL: sentinel was NOT hit — passthrough routing is broken
```

---

## Why This Proves Traffic Is Not Hitting the Real Anthropic API

1. **The sentinel URL replaces the real Anthropic URL entirely.** The proxy's
   `AnthropicUpstreamURL` is set to `sentinel.URL()` (a local `httptest.Server`
   address like `http://127.0.0.1:XXXXX`). The string `https://api.anthropic.com`
   is never referenced anywhere in the harness.

2. **The sentinel marker is unique and non-guessable.** `DETOUR_HARNESS_SENTINEL_TRAFFIC_LEAK_DETECTED`
   will never appear in an Ollama response because the test prompt does not
   request it and no small model generates this phrase spontaneously.

3. **Hit counting is atomic.** `sync/atomic.Int64` ensures the count is accurate
   under concurrent requests — no race conditions.

4. **Test 2 is the positive control.** It explicitly proves the passthrough path
   works by verifying the sentinel IS hit for a non-local model. If the harness
   itself were broken, this test would fail too.

5. **The adapter is isolated.** Only the adapter talks to real Ollama. The proxy
   only sees local HTTP servers.

---

## Recommended Test Model

`qwen2:0.5b` (394 MB, very fast, runs on any machine with 512 MB of RAM)

To use a different model, pass `--model-name <tag>` where `<tag>` is the exact
tag shown by `ollama list`.
