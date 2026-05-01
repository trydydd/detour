package forward

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

var allowedHeaders = []string{
	"Content-Type",
	"Authorization",
	"Anthropic-Version",
	"Anthropic-Beta",
}

// Do forwards r to targetURL and writes the upstream response into w.
// It supports both streaming (text/event-stream) and non-streaming responses.
func Do(w http.ResponseWriter, r *http.Request, targetURL string) {
	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "proxy_error", err.Error())
		return
	}

	for _, h := range allowedHeaders {
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

	// Copy response headers.
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		copyStreaming(w, resp.Body)
		return
	}
	io.Copy(w, resp.Body)
}

// copyStreaming copies body to w flushing after each write.
func copyStreaming(w http.ResponseWriter, body io.Reader) {
	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			break
		}
	}
}

func writeError(w http.ResponseWriter, status int, errType, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"type": "error",
		"error": map[string]string{
			"type":    errType,
			"message": msg,
		},
	})
}
