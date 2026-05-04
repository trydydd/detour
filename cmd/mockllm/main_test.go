package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleMessagesStringContent(t *testing.T) {
	body := `{"model":"detour-mock","messages":[{"role":"user","content":"hi"}],"max_tokens":10}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handleMessages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var resp messagesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Model != "detour-mock" {
		t.Errorf("model: want detour-mock, got %q", resp.Model)
	}
	if resp.Type != "message" || resp.Role != "assistant" || resp.StopReason != "end_turn" {
		t.Errorf("envelope wrong: %+v", resp)
	}
	block, ok := resp.Content[0].(map[string]any)
	if !ok {
		t.Fatalf("content[0] not object: %T", resp.Content[0])
	}
	if block["text"] != MockReply {
		t.Errorf("text: want %q, got %v", MockReply, block["text"])
	}
}

func TestHandleMessagesArrayContent(t *testing.T) {
	// Anthropic clients (including Claude Code) send content as content blocks.
	body := `{"model":"detour-mock","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}],"max_tokens":10}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handleMessages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), MockReply) {
		t.Errorf("response missing canned reply: %s", rec.Body.String())
	}
}

func TestHandleMessagesInvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	handleMessages(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(body["error"], "invalid JSON") {
		t.Errorf("error: want 'invalid JSON', got %q", body["error"])
	}
}

func TestHandleMessagesMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	rec := httptest.NewRecorder()
	handleMessages(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status: want 405, got %d", rec.Code)
	}
}

func TestHandleMessagesDefaultModel(t *testing.T) {
	body := `{"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handleMessages(rec, req)

	var resp messagesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Model != *modelName {
		t.Errorf("default model: want %q, got %q", *modelName, resp.Model)
	}
}

func TestHandleModels(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	handleModels(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: want 200, got %d", rec.Code)
	}
	var resp modelsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Object != "list" {
		t.Errorf("object: want list, got %q", resp.Object)
	}
	if len(resp.Data) == 0 {
		t.Error("models list is empty")
	}
}

func TestHandleModelsMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/models", nil)
	rec := httptest.NewRecorder()
	handleModels(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status: want 405, got %d", rec.Code)
	}
}

func TestHandleRoot(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/something/unknown", nil)
	rec := httptest.NewRecorder()
	handleRoot(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: want 404, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "mockllm: not found") {
		t.Errorf("body: %q", rec.Body.String())
	}
}
