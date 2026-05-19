package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPatchAndRestore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	restore, err := Patch("http://localhost:8888")
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}

	path := filepath.Join(dir, ".claude", "settings.json")
	checkEnv(t, path, "ANTHROPIC_BASE_URL", "http://localhost:8888")

	restore()
	checkAbsent(t, path, "ANTHROPIC_BASE_URL")
}

func TestPatchPreservesExistingKeys(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	// Write a settings.json with an existing env key.
	p := filepath.Join(dir, ".claude", "settings.json")
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(`{"env":{"ANTHROPIC_DEFAULT_HAIKU_MODEL":"red"},"model":"opus"}`), 0o644)

	restore, err := Patch("http://localhost:8888")
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}

	checkEnv(t, p, "ANTHROPIC_DEFAULT_HAIKU_MODEL", "red")
	checkEnv(t, p, "ANTHROPIC_BASE_URL", "http://localhost:8888")

	restore()
	checkAbsent(t, p, "ANTHROPIC_BASE_URL")
	checkEnv(t, p, "ANTHROPIC_DEFAULT_HAIKU_MODEL", "red")

	// Non-env keys preserved.
	var doc map[string]any
	b, _ := os.ReadFile(p)
	json.Unmarshal(b, &doc)
	if doc["model"] != "opus" {
		t.Errorf("model key should be preserved, got %v", doc["model"])
	}
}

func TestPatchOverPreviousValue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	p := filepath.Join(dir, ".claude", "settings.json")
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(`{"env":{"ANTHROPIC_BASE_URL":"http://old"}}`), 0o644)

	restore, err := Patch("http://localhost:8888")
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	checkEnv(t, p, "ANTHROPIC_BASE_URL", "http://localhost:8888")

	restore()
	checkEnv(t, p, "ANTHROPIC_BASE_URL", "http://old")
}

func checkEnv(t *testing.T, path, key, want string) {
	t.Helper()
	var doc map[string]any
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	json.Unmarshal(b, &doc)
	env, _ := doc["env"].(map[string]any)
	if got, _ := env[key].(string); got != want {
		t.Errorf("env[%s]: want %q, got %q", key, want, got)
	}
}

func checkAbsent(t *testing.T, path, key string) {
	t.Helper()
	var doc map[string]any
	b, _ := os.ReadFile(path)
	json.Unmarshal(b, &doc)
	env, _ := doc["env"].(map[string]any)
	if _, ok := env[key]; ok {
		t.Errorf("env[%s] should be absent after restore", key)
	}
}
