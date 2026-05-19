package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const key = "ANTHROPIC_BASE_URL"

// Patch adds ANTHROPIC_BASE_URL to ~/.claude/settings.json so Claude Code
// routes API calls through the detour proxy. Returns a restore function that
// removes the key when called. Safe to call even if the file doesn't exist.
func Patch(proxyURL string) (restore func(), err error) {
	path, err := claudeSettingsPath()
	if err != nil {
		return noop, err
	}

	original, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return noop, err
	}

	var doc map[string]any
	if len(original) > 0 {
		if err := json.Unmarshal(original, &doc); err != nil {
			return noop, err
		}
	} else {
		doc = map[string]any{}
	}

	// Record the previous value (may be absent).
	env := envSection(doc)
	prev, hadPrev := env[key]

	// Write the proxy URL.
	env[key] = proxyURL
	doc["env"] = env
	if err := writeJSON(path, doc); err != nil {
		return noop, err
	}

	restore = func() {
		var d map[string]any
		if b, err := os.ReadFile(path); err == nil {
			json.Unmarshal(b, &d)
		}
		if d == nil {
			d = map[string]any{}
		}
		e := envSection(d)
		if hadPrev {
			e[key] = prev
		} else {
			delete(e, key)
		}
		if len(e) == 0 {
			delete(d, "env")
		} else {
			d["env"] = e
		}
		writeJSON(path, d)
	}
	return restore, nil
}

func claudeSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

func envSection(doc map[string]any) map[string]any {
	if v, ok := doc["env"]; ok {
		if m, ok := v.(map[string]any); ok {
			return m
		}
	}
	return map[string]any{}
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func noop() {}
