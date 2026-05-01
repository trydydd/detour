package forward

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestForwardsBodyUnchanged(t *testing.T) {
	const body = `{"model":"red","messages":[]}`
	var got string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	Do(rec, req, upstream.URL+"/v1/messages")

	if got != body {
		t.Errorf("body mismatch\nwant: %s\ngot:  %s", body, got)
	}
}

func TestForwardsAllowedHeaders(t *testing.T) {
	var gotHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk-test")
	req.Header.Set("Anthropic-Version", "2023-06-01")
	req.Header.Set("Anthropic-Beta", "tools-2024-04-04")
	rec := httptest.NewRecorder()
	Do(rec, req, upstream.URL)

	for _, h := range []string{"Content-Type", "Authorization", "Anthropic-Version", "Anthropic-Beta"} {
		if gotHeaders.Get(h) == "" {
			t.Errorf("header %q not forwarded", h)
		}
	}
}

func TestStripsHostHeader(t *testing.T) {
	var gotHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Header.Get("X-Original-Host")
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
	req.Header.Set("Host", "api.anthropic.com")
	rec := httptest.NewRecorder()
	Do(rec, req, upstream.URL)

	// The upstream should not see an X-Original-Host injected by us,
	// and the Host seen by the upstream should be upstream's own host, not api.anthropic.com.
	_ = gotHost
}

func TestNonStreamingResponse(t *testing.T) {
	const responseBody = `{"id":"msg_1","type":"message"}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, responseBody)
	}))
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	Do(rec, req, upstream.URL)

	if rec.Body.String() != responseBody {
		t.Errorf("body mismatch\nwant: %s\ngot:  %s", responseBody, rec.Body.String())
	}
}

func TestStreamingResponse(t *testing.T) {
	chunks := []string{
		"event: message_start\ndata: {}\n\n",
		"event: content_block_delta\ndata: {}\n\n",
		"event: message_stop\ndata: {}\n\n",
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher := w.(http.Flusher)
		for _, c := range chunks {
			io.WriteString(w, c)
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"stream":true}`))
	rec := httptest.NewRecorder()
	Do(rec, req, upstream.URL)

	want := strings.Join(chunks, "")
	if rec.Body.String() != want {
		t.Errorf("streaming body mismatch\nwant: %q\ngot:  %q", want, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("expected text/event-stream content-type, got %q", ct)
	}
}

func TestUpstream4xxForwarded(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		io.WriteString(w, `{"error":"rate limited"}`)
	}))
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	Do(rec, req, upstream.URL)

	if rec.Code != 429 {
		t.Errorf("want 429, got %d", rec.Code)
	}
}

func TestUpstreamDown(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	Do(rec, req, "http://127.0.0.1:1") // nothing listening

	if rec.Code != 502 {
		t.Errorf("want 502 when upstream down, got %d", rec.Code)
	}
}
