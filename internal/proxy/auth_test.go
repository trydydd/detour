package proxy

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

const testAuthToken = "test-secret-token-123"

func init() {
	// Set the expected auth token for all tests in this package
	// This allows tests to proceed past auth checks
	os.Setenv("ANTHROPIC_DETOUR_AUTH", testAuthToken)
}

func TestAuthRequired(t *testing.T) {
	// C2: Proxy requires authentication header on all requests
	cfg := &Config{
		ModelName:            "red",
		LocalUpstreamURL:     "http://127.0.0.1:8000",
		AnthropicUpstreamURL: "https://api.anthropic.com",
	}
	mux := NewMux(cfg)

	// Test without auth token
	req := httptest.NewRequest("POST", "/v1/messages", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("request without auth token: got status %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthValidToken(t *testing.T) {
	// C2: Request with valid auth token proceeds normally
	cfg := &Config{
		ModelName:            "red",
		LocalUpstreamURL:     "http://127.0.0.1:8000",
		AnthropicUpstreamURL: "https://api.anthropic.com",
	}
	mux := NewMux(cfg)

	// Test with valid auth token
	req := httptest.NewRequest("POST", "/v1/messages", nil)
	req.Header.Set("X-Detour-Auth", testAuthToken)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should not return 401 - may fail for other reasons (upstream unavailable)
	// but should not be rejected due to auth
	if w.Code == http.StatusUnauthorized {
		t.Errorf("request with valid auth token: got 401 Unauthorized, expected to proceed")
	}
}

func TestAuthInvalidToken(t *testing.T) {
	// C2: Request with invalid auth token returns 401
	cfg := &Config{
		ModelName:            "red",
		LocalUpstreamURL:     "http://127.0.0.1:8000",
		AnthropicUpstreamURL: "https://api.anthropic.com",
	}
	mux := NewMux(cfg)

	// Test with invalid auth token
	req := httptest.NewRequest("POST", "/v1/messages", nil)
	req.Header.Set("X-Detour-Auth", "wrong-token")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("request with invalid auth token: got status %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthHealthEndpointSkipped(t *testing.T) {
	// Health endpoint should not require auth
	cfg := &Config{
		ModelName:            "red",
		LocalUpstreamURL:     "http://127.0.0.1:8000",
		AnthropicUpstreamURL: "https://api.anthropic.com",
	}
	mux := NewMux(cfg)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("health endpoint without auth: got status %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAuthDeniedWhenTokenNotConfigured(t *testing.T) {
	// C2: proxy must deny requests when ANTHROPIC_DETOUR_AUTH is not set (no silent bypass)
	orig := os.Getenv("ANTHROPIC_DETOUR_AUTH")
	os.Unsetenv("ANTHROPIC_DETOUR_AUTH")
	defer os.Setenv("ANTHROPIC_DETOUR_AUTH", orig)

	cfg := &Config{
		ModelName:            "red",
		LocalUpstreamURL:     "http://127.0.0.1:8000",
		AnthropicUpstreamURL: "https://api.anthropic.com",
	}
	mux := NewMux(cfg)

	req := httptest.NewRequest("POST", "/v1/messages", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("unconfigured proxy: got status %d, want %d", w.Code, http.StatusUnauthorized)
	}
}
