package forward

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
)

var allowedHeaders = []string{
	"Content-Type",
	"X-Api-Key",
	"Authorization",
	"Anthropic-Version",
	"Anthropic-Beta",
}

var allowedLocalHeaders = []string{
	"Content-Type",
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

// DoLocal is like Do but strips thinking blocks from the response.
// Local inference servers cannot produce valid Anthropic thinking signatures;
// any thinking blocks they return would cause subsequent passthrough requests
// to Anthropic to fail with a 400 invalid-signature error.
func DoLocal(w http.ResponseWriter, r *http.Request, targetURL string) {
	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "proxy_error", err.Error())
		return
	}
	for _, h := range allowedLocalHeaders {
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

	// Copy response headers, skipping Content-Length and Transfer-Encoding
	// since we may change the body size by stripping thinking blocks.
	for k, vv := range resp.Header {
		if strings.EqualFold(k, "content-length") || strings.EqualFold(k, "transfer-encoding") {
			continue
		}
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}

	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		w.WriteHeader(resp.StatusCode)
		copyStreamingStripThinking(w, resp.Body)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	body = stripThinkingFromResponseBody(body)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

// stripThinkingFromResponseBody removes content blocks with type "thinking"
// from a non-streaming Anthropic Messages API response body.
func stripThinkingFromResponseBody(body []byte) []byte {
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(body, &resp); err != nil {
		return body
	}
	contentRaw, ok := resp["content"]
	if !ok {
		return body
	}
	var content []json.RawMessage
	if err := json.Unmarshal(contentRaw, &content); err != nil {
		return body
	}
	filtered := make([]json.RawMessage, 0, len(content))
	changed := false
	for _, block := range content {
		var b struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(block, &b) == nil && b.Type == "thinking" {
			changed = true
			continue
		}
		filtered = append(filtered, block)
	}
	if !changed {
		return body
	}
	newContent, err := json.Marshal(filtered)
	if err != nil {
		return body
	}
	resp["content"] = json.RawMessage(newContent)
	out, err := json.Marshal(resp)
	if err != nil {
		return body
	}
	return out
}

// copyStreamingStripThinking copies an SSE body to w, dropping events that
// belong to thinking content blocks. message_start events are also patched to
// add type:"message" and role:"assistant" when the upstream omits them — some
// servers (e.g. vLLM) leave these off, which breaks downstream consumers that
// require them (notably Claude Code's mobile transcript relay).
func copyStreamingStripThinking(w http.ResponseWriter, body io.Reader) {
	flusher, canFlush := w.(http.Flusher)
	thinkingIdx := make(map[int]bool)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	var event []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if !shouldDropSSEEvent(event, thinkingIdx) {
				for _, l := range patchEventLines(event) {
					io.WriteString(w, l+"\n")
				}
				io.WriteString(w, "\n")
				if canFlush {
					flusher.Flush()
				}
			}
			event = nil
		} else {
			event = append(event, line)
		}
	}
	// Flush any trailing lines (malformed stream without final blank line).
	if len(event) > 0 {
		for _, l := range patchEventLines(event) {
			io.WriteString(w, l+"\n")
		}
		if canFlush {
			flusher.Flush()
		}
	}
}

// patchEventLines rewrites the data: payload of message_start events to inject
// Anthropic-required fields when missing. All other lines pass through.
func patchEventLines(lines []string) []string {
	for i, l := range lines {
		const prefix = "data:"
		if !strings.HasPrefix(l, prefix) {
			continue
		}
		data := strings.TrimSpace(l[len(prefix):])
		patched, ok := patchMessageStart([]byte(data))
		if !ok {
			continue
		}
		lines[i] = "data: " + string(patched)
	}
	return lines
}

// patchMessageStart adds type:"message" and role:"assistant" to the inner
// message object of a message_start event when missing. Returns the original
// bytes and false when the data is not a message_start event or both fields
// are already present.
func patchMessageStart(data []byte) ([]byte, bool) {
	var ev map[string]json.RawMessage
	if err := json.Unmarshal(data, &ev); err != nil {
		return data, false
	}
	typRaw, ok := ev["type"]
	if !ok {
		return data, false
	}
	var typ string
	if err := json.Unmarshal(typRaw, &typ); err != nil || typ != "message_start" {
		return data, false
	}
	msgRaw, ok := ev["message"]
	if !ok {
		return data, false
	}
	var msg map[string]json.RawMessage
	if err := json.Unmarshal(msgRaw, &msg); err != nil {
		return data, false
	}
	changed := false
	if _, ok := msg["type"]; !ok {
		msg["type"] = json.RawMessage(`"message"`)
		changed = true
	}
	if _, ok := msg["role"]; !ok {
		msg["role"] = json.RawMessage(`"assistant"`)
		changed = true
	}
	if !changed {
		return data, false
	}
	newMsg, err := json.Marshal(msg)
	if err != nil {
		return data, false
	}
	ev["message"] = json.RawMessage(newMsg)
	out, err := json.Marshal(ev)
	if err != nil {
		return data, false
	}
	return out, true
}

// shouldDropSSEEvent returns true for content_block_* events belonging to a
// thinking block. It also records newly-seen thinking block indices.
func shouldDropSSEEvent(lines []string, thinkingIdx map[int]bool) bool {
	for _, l := range lines {
		if !strings.HasPrefix(l, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(l, "data:"))
		if data == "[DONE]" {
			return false
		}
		var ev struct {
			Type         string `json:"type"`
			Index        int    `json:"index"`
			ContentBlock struct {
				Type string `json:"type"`
			} `json:"content_block"`
		}
		if json.Unmarshal([]byte(data), &ev) != nil {
			return false
		}
		switch ev.Type {
		case "content_block_start":
			if ev.ContentBlock.Type == "thinking" {
				thinkingIdx[ev.Index] = true
				return true
			}
		case "content_block_delta", "content_block_stop":
			return thinkingIdx[ev.Index]
		}
		return false
	}
	return false
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
