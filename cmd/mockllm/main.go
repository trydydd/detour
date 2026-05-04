package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const MockReply = "THIS IS DETOUR TEST!"

var (
	port      = flag.Int("port", 9999, "Port to listen on")
	modelName = flag.String("model-name", "detour-mock", "Model tag advertised by /v1/models")
	host      = flag.String("host", "127.0.0.1", "Bind address")
)

// Anthropic API request shapes
type messagesRequest struct {
	Model     string    `json:"model"`
	Messages  []message `json:"messages"`
	MaxTokens int       `json:"max_tokens"`
	Stream    bool      `json:"stream"`
}

type message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// Anthropic API response shapes
type messagesResponse struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Role          string `json:"role"`
	Content       []any  `json:"content"`
	Model         string `json:"model"`
	StopReason    string `json:"stop_reason"`
	StopSequence  any    `json:"stop_sequence"`
	Usage         usage  `json:"usage"`
}

type usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type modelsResponse struct {
	Object string       `json:"object"`
	Data   []modelEntry `json:"data"`
}

type modelEntry struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Created     int64  `json:"created"`
	DisplayName string `json:"display_name"`
	OwnedBy     string `json:"owned_by"`
}

func handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read request body: "+err.Error())
		return
	}

	var req messagesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	model := req.Model
	if model == "" {
		model = *modelName
	}

	if req.Stream {
		streamMockReply(w, model)
		return
	}

	resp := messagesResponse{
		ID:           "msg_mock_001",
		Type:         "message",
		Role:         "assistant",
		Content: []any{
			map[string]any{
				"type": "text",
				"text": MockReply,
			},
		},
		Model:        model,
		StopReason:   "end_turn",
		StopSequence: nil,
		Usage: usage{
			InputTokens:  1,
			OutputTokens: 1,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// streamMockReply emits an Anthropic-spec SSE sequence with the canned reply.
// The message_start payload includes type:"message" and role:"assistant" — the
// fields some upstream servers (e.g. vLLM) omit, which breaks the Claude Code
// mobile transcript relay.
func streamMockReply(w http.ResponseWriter, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)
	send := func(event string, data any) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		if flusher != nil {
			flusher.Flush()
		}
	}

	send("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            "msg_mock_001",
			"type":          "message",
			"role":          "assistant",
			"content":       []any{},
			"model":         model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": 1, "output_tokens": 0},
		},
	})
	send("content_block_start", map[string]any{
		"type":          "content_block_start",
		"index":         0,
		"content_block": map[string]any{"type": "text", "text": ""},
	})
	send("content_block_delta", map[string]any{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]any{"type": "text_delta", "text": MockReply},
	})
	send("content_block_stop", map[string]any{
		"type":  "content_block_stop",
		"index": 0,
	})
	send("message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": map[string]any{"output_tokens": 1},
	})
	send("message_stop", map[string]any{"type": "message_stop"})
}

func handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	models := []modelEntry{
		{
			ID:          "claude-3-5-sonnet-20241022",
			Type:        "model",
			Created:     time.Now().Unix(),
			DisplayName: "Claude 3.5 Sonnet",
			OwnedBy:     "anthropic",
		},
		{
			ID:          "claude-3-opus-20250219",
			Type:        "model",
			Created:     time.Now().Unix(),
			DisplayName: "Claude 3 Opus",
			OwnedBy:     "anthropic",
		},
		{
			ID:          "claude-3-haiku-20240307",
			Type:        "model",
			Created:     time.Now().Unix(),
			DisplayName: "Claude 3 Haiku",
			OwnedBy:     "anthropic",
		},
	}

	resp := modelsResponse{
		Object: "list",
		Data:   models,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprint(w, "mockllm: not found\n")
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func main() {
	flag.Parse()

	addr := fmt.Sprintf("%s:%d", *host, *port)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/messages", handleMessages)
	mux.HandleFunc("/v1/models", handleModels)
	mux.HandleFunc("/", handleRoot)

	log.Printf("mockllm listening on %s", addr)
	log.Printf("model name: %s", *modelName)
	log.Printf("canned reply: %s", MockReply)
	log.Printf("hint: ANTHROPIC_API_KEY=test go run ./cmd/mockllm --port %d", *port)

	log.Fatal(http.ListenAndServe(addr, mux))
}
