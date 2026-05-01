package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

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
	}
	return mux
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
		}

		r.Body = io.NopCloser(bytes.NewReader(body))
		forward.Do(w, r, targetURL)
	}
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

