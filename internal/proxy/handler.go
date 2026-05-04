package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/trydydd/detour/internal/forward"
	"github.com/trydydd/detour/internal/router"
)

const version = "0.1.0"
const maxRequestBytes = 10 << 20 // 10 MiB

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
		mux.HandleFunc("/v1/messages", maybeLog(makeMessagesHandler(cfg)))
		mux.HandleFunc("/v1/models", maybeLog(makeModelsHandler(cfg)))
		mux.HandleFunc("/", maybeLog(makePassthroughHandler(cfg)))
	}
	return mux
}

// makeModelsHandler proxies /v1/models to Anthropic unchanged.
func makeModelsHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		forward.Do(w, r, cfg.AnthropicUpstreamURL+"/v1/models")
	}
}

// makePassthroughHandler proxies any unrecognised path to Anthropic unchanged.
func makePassthroughHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		forward.Do(w, r, cfg.AnthropicUpstreamURL+r.URL.RequestURI())
	}
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
		body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes+1))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "could not read request body")
			return
		}
		if len(body) > maxRequestBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "invalid_request", "request body too large")
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
