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
