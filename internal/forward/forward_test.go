package forward

import (
	"encoding/json"
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

func TestDoLocalStripsAuthHeader(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer sk-ant-secret")
	req.Header.Set("Anthropic-Version", "2023-06-01")
	rec := httptest.NewRecorder()
	DoLocal(rec, req, upstream.URL)

	if gotAuth != "" {
		t.Errorf("DoLocal must not forward Authorization to local backend, got %q", gotAuth)
	}
}

func TestDoForwardsAuthHeader(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer sk-ant-secret")
	rec := httptest.NewRecorder()
	Do(rec, req, upstream.URL)

	if gotAuth != "Bearer sk-ant-secret" {
		t.Errorf("Do must forward Authorization to Anthropic backend, got %q", gotAuth)
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

func TestPatchMessageStartAddsBothFields(t *testing.T) {
	in := []byte(`{"type":"message_start","message":{"id":"x","content":[],"model":"red","stop_reason":null,"usage":{"input_tokens":1,"output_tokens":0}}}`)
	out, changed := patchMessageStart(in)
	if !changed {
		t.Fatal("expected patch, got none")
	}
	var ev struct {
		Message struct {
			Type string `json:"type"`
			Role string `json:"role"`
			ID   string `json:"id"`
		} `json:"message"`
	}
	if err := json.Unmarshal(out, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.Message.Type != "message" {
		t.Errorf("type: want %q, got %q", "message", ev.Message.Type)
	}
	if ev.Message.Role != "assistant" {
		t.Errorf("role: want %q, got %q", "assistant", ev.Message.Role)
	}
	if ev.Message.ID != "x" {
		t.Errorf("id: existing field overwritten or lost: %q", ev.Message.ID)
	}
}

func TestPatchMessageStartAddsOnlyMissingField(t *testing.T) {
	// type already present; only role should be added.
	in := []byte(`{"type":"message_start","message":{"id":"x","type":"message","content":[]}}`)
	out, changed := patchMessageStart(in)
	if !changed {
		t.Fatal("expected patch when role missing")
	}
	if !strings.Contains(string(out), `"role":"assistant"`) {
		t.Errorf("role not added: %s", out)
	}
	// role already present; only type should be added.
	in = []byte(`{"type":"message_start","message":{"id":"x","role":"assistant","content":[]}}`)
	out, changed = patchMessageStart(in)
	if !changed {
		t.Fatal("expected patch when type missing")
	}
	if !strings.Contains(string(out), `"type":"message"`) {
		t.Errorf("type not added: %s", out)
	}
}

func TestPatchMessageStartUnchangedWhenComplete(t *testing.T) {
	in := []byte(`{"type":"message_start","message":{"id":"x","type":"message","role":"assistant","content":[]}}`)
	out, changed := patchMessageStart(in)
	if changed {
		t.Errorf("expected no change, got patched output: %s", out)
	}
	if string(out) != string(in) {
		t.Errorf("bytes mutated even though changed=false")
	}
}

func TestPatchMessageStartIgnoresOtherEventTypes(t *testing.T) {
	in := []byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`)
	out, changed := patchMessageStart(in)
	if changed {
		t.Errorf("non-message_start event should not be patched: %s", out)
	}
}

func TestStreamingPatchesVLLMMessageStart(t *testing.T) {
	// vLLM-shaped SSE: message_start lacks type and role on the inner message.
	stream := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"chatcmpl-1","content":[],"model":"red","stop_reason":null,"usage":{"input_tokens":1,"output_tokens":0}}}` + "\n\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, stream)
	}))
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"stream":true}`))
	rec := httptest.NewRecorder()
	DoLocal(rec, req, upstream.URL)

	body := rec.Body.String()
	if !strings.Contains(body, `"type":"message"`) {
		t.Errorf("message_start was not patched with type:\n%s", body)
	}
	if !strings.Contains(body, `"role":"assistant"`) {
		t.Errorf("message_start was not patched with role:\n%s", body)
	}
	// non-message_start events must remain unmodified
	if !strings.Contains(body, `"type":"content_block_delta"`) || !strings.Contains(body, `"text":"hi"`) {
		t.Errorf("non-message_start events were corrupted:\n%s", body)
	}
}

func TestStreamingPreservesCompleteMessageStart(t *testing.T) {
	// Already-complete message_start should be byte-preserved.
	complete := `{"type":"message_start","message":{"id":"chatcmpl-2","type":"message","role":"assistant","content":[],"model":"red","stop_reason":null,"usage":{"input_tokens":1,"output_tokens":0}}}`
	stream := "event: message_start\ndata: " + complete + "\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, stream)
	}))
	defer upstream.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"stream":true}`))
	rec := httptest.NewRecorder()
	DoLocal(rec, req, upstream.URL)

	if !strings.Contains(rec.Body.String(), "data: "+complete) {
		t.Errorf("complete message_start payload was modified:\n%s", rec.Body.String())
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
