package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/trydydd/detour/internal/forward"
	"github.com/trydydd/detour/internal/router"
)

const version = "0.1.0"

// Config holds the proxy routing configuration.
type Config struct {
	ModelName            string
	LocalUpstreamURL     string
	AnthropicUpstreamURL string
}

func NewMux(cfg *Config) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	if cfg != nil {
		mux.HandleFunc("/v1/messages", makeMessagesHandler(cfg))
		mux.HandleFunc("/v1/models", makeModelsHandler(cfg))
		mux.HandleFunc("/", makePassthroughHandler(cfg))
	}
	return mux
}

// makeModelsHandler proxies GET /v1/models to Anthropic and injects the local
// model into the response so Claude Code recognises it and displays its name.
func makeModelsHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		outReq, err := http.NewRequestWithContext(r.Context(), r.Method,
			cfg.AnthropicUpstreamURL+"/v1/models", r.Body)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "proxy_error", err.Error())
			return
		}
		for _, h := range []string{"Authorization", "Anthropic-Version", "Anthropic-Beta"} {
			if v := r.Header.Get(h); v != "" {
				outReq.Header.Set(h, v)
			}
		}
		resp, err := http.DefaultClient.Do(outReq)
		if err != nil {
			writeError(w, http.StatusBadGateway, "proxy_error", "upstream unavailable: "+err.Error())
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			writeError(w, http.StatusBadGateway, "proxy_error", "failed to read response")
			return
		}
		if resp.StatusCode == http.StatusOK {
			body = injectLocalModel(body, cfg.ModelName)
		}

		for k, vv := range resp.Header {
			if strings.EqualFold(k, "content-length") || strings.EqualFold(k, "transfer-encoding") {
				continue
			}
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
	}
}

// makePassthroughHandler proxies any unrecognised path to Anthropic unchanged.
func makePassthroughHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		forward.Do(w, r, cfg.AnthropicUpstreamURL+r.URL.RequestURI())
	}
}

// injectLocalModel prepends the configured local model to an Anthropic models
// list response so Claude Code recognises the alias and displays it by name.
func injectLocalModel(body []byte, modelName string) []byte {
	var list map[string]json.RawMessage
	if err := json.Unmarshal(body, &list); err != nil {
		return body
	}
	dataRaw, ok := list["data"]
	if !ok {
		return body
	}
	var data []json.RawMessage
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return body
	}
	entry, err := json.Marshal(map[string]any{
		"type":         "model",
		"id":           modelName,
		"display_name": modelName,
		"created_at":   time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return body
	}
	data = append([]json.RawMessage{entry}, data...)
	newData, err := json.Marshal(data)
	if err != nil {
		return body
	}
	list["data"] = newData
	out, err := json.Marshal(list)
	if err != nil {
		return body
	}
	return out
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": version,
	})
}

func makeMessagesHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "could not read request body")
			return
		}

		model, err := peekModel(body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		if model == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "missing required field: model")
			return
		}

		backend := router.Route(model, cfg.ModelName)
		var targetURL string
		if backend == "passthrough" {
			targetURL = cfg.AnthropicUpstreamURL + "/v1/messages"
		} else {
			targetURL = cfg.LocalUpstreamURL + "/v1/messages"
			// Local inference servers don't sign thinking blocks. Strip thinking
			// from the request so the model never generates blocks with invalid
			// signatures that would break subsequent passthrough requests.
			body = stripThinkingFromBody(body)
			if v := r.Header.Get("Anthropic-Beta"); v != "" {
				r.Header.Set("Anthropic-Beta", filterThinkingBeta(v))
			}
		}

		r.Body = io.NopCloser(bytes.NewReader(body))
		if backend == "passthrough" {
			forward.Do(w, r, targetURL)
		} else {
			forward.DoLocal(w, r, targetURL)
		}
	}
}

// stripThinkingFromBody removes the "thinking" field from a JSON request body.
func stripThinkingFromBody(body []byte) []byte {
	var req map[string]json.RawMessage
	if err := json.Unmarshal(body, &req); err != nil {
		return body
	}
	if _, ok := req["thinking"]; !ok {
		return body
	}
	delete(req, "thinking")
	out, err := json.Marshal(req)
	if err != nil {
		return body
	}
	return out
}

// filterThinkingBeta removes thinking-related tokens from an Anthropic-Beta header value.
func filterThinkingBeta(v string) string {
	parts := strings.Split(v, ",")
	kept := parts[:0]
	for _, p := range parts {
		if !strings.Contains(strings.TrimSpace(p), "thinking") {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, ",")
}

type peekRequest struct {
	Model string `json:"model"`
}

func peekModel(body []byte) (string, error) {
	var pr peekRequest
	if err := json.Unmarshal(body, &pr); err != nil {
		return "", err
	}
	return pr.Model, nil
}

