package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testConfig(localUpstream, passthroughUpstream string) *Config {
	return &Config{
		ModelName:           "red",
		LocalUpstreamURL:    localUpstream,
		AnthropicUpstreamURL: passthroughUpstream,
	}
}

func TestHealth(t *testing.T) {
	mux := NewMux(nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("want status=ok, got %q", body["status"])
	}
}

func TestRouteLocal(t *testing.T) {
	var gotURL string
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.Path
		io.WriteString(w, `{"id":"msg_1","type":"message"}`)
	}))
	defer local.Close()

	passthrough := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("passthrough should not be called")
	}))
	defer passthrough.Close()

	mux := NewMux(testConfig(local.URL, passthrough.URL))
	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"red","messages":[],"max_tokens":10}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if gotURL == "" {
		t.Error("local upstream was not called")
	}
}

func TestRoutePassthrough(t *testing.T) {
	var called bool
	passthrough := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		io.WriteString(w, `{"id":"msg_2","type":"message"}`)
	}))
	defer passthrough.Close()

	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("local should not be called for opus")
	}))
	defer local.Close()

	mux := NewMux(testConfig(local.URL, passthrough.URL))
	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"claude-opus-4-7","messages":[],"max_tokens":10}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if !called {
		t.Error("passthrough upstream was not called")
	}
}

func TestMissingModel(t *testing.T) {
	mux := NewMux(testConfig("http://unused", "http://unused"))
	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"messages":[]}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
	assertErrorJSON(t, rec.Body.String())
}

func TestInvalidJSON(t *testing.T) {
	mux := NewMux(testConfig("http://unused", "http://unused"))
	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`not json`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}
	assertErrorJSON(t, rec.Body.String())
}

func TestErrorFormat(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "invalid_request", "test message")

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["type"] != "error" {
		t.Errorf("want type=error, got %v", body["type"])
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field is not an object: %T", body["error"])
	}
	if errObj["type"] != "invalid_request" {
		t.Errorf("want error.type=invalid_request, got %v", errObj["type"])
	}
	if errObj["message"] != "test message" {
		t.Errorf("want error.message=test message, got %v", errObj["message"])
	}
}

func TestHealthRegistered(t *testing.T) {
	mux := NewMux(testConfig("http://unused", "http://unused"))
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("health check: want 200, got %d", rec.Code)
	}
}

func TestLocalStripsResponseThinking(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"type":"message","content":[{"type":"thinking","thinking":"hmm","signature":"fakesig"},{"type":"text","text":"Hello"}]}`)
	}))
	defer local.Close()

	mux := NewMux(testConfig(local.URL, "http://unused"))
	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"red","messages":[],"max_tokens":10}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var content []struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(resp["content"], &content); err != nil {
		t.Fatalf("decode content: %v", err)
	}
	for _, block := range content {
		if block.Type == "thinking" {
			t.Error("thinking block should be stripped from local response")
		}
	}
	if len(content) != 1 || content[0].Type != "text" {
		t.Errorf("expected only text block, got %+v", content)
	}
}

func TestLocalStripsStreamingThinking(t *testing.T) {
	sse := "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\"}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"hmm\"}}\n\n" +
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n" +
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1}\n\n"

	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, sse)
	}))
	defer local.Close()

	mux := NewMux(testConfig(local.URL, "http://unused"))
	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"red","messages":[],"max_tokens":10,"stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, "thinking") {
		t.Errorf("thinking events should be stripped from streaming response, got:\n%s", body)
	}
	if !strings.Contains(body, "text_delta") {
		t.Error("text events should be preserved in streaming response")
	}
}

func TestLocalStripsThinking(t *testing.T) {
	var gotBody []byte
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		io.WriteString(w, `{"id":"msg_1","type":"message"}`)
	}))
	defer local.Close()

	mux := NewMux(testConfig(local.URL, "http://unused"))
	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"red","messages":[],"max_tokens":10,"thinking":{"type":"enabled","budget_tokens":5000}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var body map[string]json.RawMessage
	if err := json.Unmarshal(gotBody, &body); err != nil {
		t.Fatalf("local received invalid JSON: %v", err)
	}
	if _, ok := body["thinking"]; ok {
		t.Error("thinking field should be stripped from local requests")
	}
}

func TestLocalFiltersThinkingBeta(t *testing.T) {
	var gotBeta string
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBeta = r.Header.Get("Anthropic-Beta")
		io.WriteString(w, `{"id":"msg_1","type":"message"}`)
	}))
	defer local.Close()

	mux := NewMux(testConfig(local.URL, "http://unused"))
	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"red","messages":[],"max_tokens":10}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Anthropic-Beta", "interleaved-thinking-2025-05-14,prompt-caching-2024-07-31")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if strings.Contains(gotBeta, "thinking") {
		t.Errorf("thinking beta should be stripped, got: %q", gotBeta)
	}
	if !strings.Contains(gotBeta, "prompt-caching") {
		t.Errorf("non-thinking beta should be preserved, got: %q", gotBeta)
	}
}

func TestPassthroughPreservesThinking(t *testing.T) {
	var gotBody []byte
	var gotBeta string
	passthrough := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotBeta = r.Header.Get("Anthropic-Beta")
		io.WriteString(w, `{"id":"msg_2","type":"message"}`)
	}))
	defer passthrough.Close()

	mux := NewMux(testConfig("http://unused", passthrough.URL))
	req := httptest.NewRequest(http.MethodPost, "/v1/messages",
		strings.NewReader(`{"model":"claude-opus-4-7","messages":[],"max_tokens":10,"thinking":{"type":"enabled","budget_tokens":5000}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Anthropic-Beta", "interleaved-thinking-2025-05-14")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var body map[string]json.RawMessage
	if err := json.Unmarshal(gotBody, &body); err != nil {
		t.Fatalf("passthrough received invalid JSON: %v", err)
	}
	if _, ok := body["thinking"]; !ok {
		t.Error("thinking field should be preserved for passthrough requests")
	}
	if gotBeta != "interleaved-thinking-2025-05-14" {
		t.Errorf("beta header should be preserved for passthrough, got: %q", gotBeta)
	}
}

func TestModelsHandlerForwardsXApiKey(t *testing.T) {
	var gotHeaders http.Header
	anthropic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"object":"list","data":[{"id":"claude-opus-4-7","type":"model"}]}`)
	}))
	defer anthropic.Close()

	mux := NewMux(testConfig("http://unused", anthropic.URL))
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("X-Api-Key", "test-key-123")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if gotHeaders.Get("X-Api-Key") != "test-key-123" {
		t.Errorf("x-api-key header not forwarded to Anthropic, got: %q", gotHeaders.Get("X-Api-Key"))
	}
}


func assertErrorJSON(t *testing.T, body string) {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("response is not valid JSON: %v\nbody: %s", err, body)
	}
	if v["type"] != "error" {
		t.Errorf("want type=error in error response, got %v", v["type"])
	}
}
