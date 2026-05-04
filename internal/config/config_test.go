package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()
	if cfg.Port != 8888 {
		t.Errorf("default port: want 8888, got %d", cfg.Port)
	}
}

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	orig := &Config{
		Port:      9000,
		ModelName: "red",
		ModelAPI:  "http://192.168.0.28:8000",
	}
	if err := orig.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if *loaded != *orig {
		t.Errorf("round-trip mismatch:\n  saved:  %+v\n  loaded: %+v", orig, loaded)
	}
}

func TestLoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load on missing file should not error, got: %v", err)
	}
	cfg.applyDefaults()
	if cfg.Port != 8888 {
		t.Errorf("want default port 8888, got %d", cfg.Port)
	}
}

func TestFlagsOverrideSaved(t *testing.T) {
	dir := t.TempDir()
	saved := &Config{Port: 9000, ModelName: "saved", ModelAPI: "http://saved:8000"}
	if err := saved.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Simulate flags arriving with a different port and model name.
	flags := &Config{Port: 7777, ModelName: "flags", ModelAPI: "http://flags:8000"}
	merged := merge(saved, flags)
	if merged.Port != 7777 {
		t.Errorf("flag port should win: want 7777, got %d", merged.Port)
	}
	if merged.ModelName != "flags" {
		t.Errorf("flag model name should win: want flags, got %q", merged.ModelName)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		ok   bool
	}{
		{"valid http", Config{Port: 8888, ModelName: "red", ModelAPI: "http://localhost"}, true},
		{"valid https", Config{Port: 8888, ModelName: "red", ModelAPI: "https://example.com"}, true},
		{"valid with port", Config{Port: 8888, ModelName: "red", ModelAPI: "http://127.0.0.1:8000"}, true},
		{"missing ModelName", Config{Port: 8888, ModelAPI: "http://x"}, false},
		{"missing ModelAPI", Config{Port: 8888, ModelName: "red"}, false},
		{"missing both", Config{Port: 8888}, false},
		{"invalid scheme file", Config{Port: 8888, ModelName: "red", ModelAPI: "file:///etc/passwd"}, false},
		{"invalid scheme ftp", Config{Port: 8888, ModelName: "red", ModelAPI: "ftp://example.com"}, false},
		{"invalid scheme websocket", Config{Port: 8888, ModelName: "red", ModelAPI: "ws://example.com"}, false},
		{"invalid with path", Config{Port: 8888, ModelName: "red", ModelAPI: "http://localhost/v1"}, false},
		{"invalid with query", Config{Port: 8888, ModelName: "red", ModelAPI: "http://localhost?foo=bar"}, false},
		{"invalid with fragment", Config{Port: 8888, ModelName: "red", ModelAPI: "http://localhost#section"}, false},
		{"invalid empty host", Config{Port: 8888, ModelName: "red", ModelAPI: "http://"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.ok && err != nil {
				t.Errorf("expected valid, got: %v", err)
			}
			if !tc.ok && err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestSaveFilePermissions(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{ModelName: "x", ModelAPI: "http://x:8000", Port: 8888}
	if err := cfg.Save(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("stat config.json: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("config.json permissions: want 0600, got %04o", got)
	}
}

func TestLoadRejectsInsecurePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	// Write a valid config with world-readable permissions
	data := `{"port":8888,"model_name":"x","model_api":"http://x:8000"}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Error("Load should reject config.json with 0644 permissions")
	}
}

func TestLoadAcceptsSecurePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{"port":8888,"model_name":"x","model_api":"http://x:8000"}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err != nil {
		t.Errorf("Load should accept config.json with 0600 permissions: %v", err)
	}
}

func TestCleartextUpstreamWarning(t *testing.T) {
	cases := []struct {
		modelAPI string
		wantWarn bool
	}{
		{"http://192.168.0.28:8000", true},
		{"http://10.0.0.5", true},
		{"http://remote-server:8080", true},
		{"http://localhost", false},
		{"http://localhost:8000", false},
		{"http://127.0.0.1", false},
		{"http://127.0.0.1:8000", false},
		{"https://192.168.0.28:8000", false}, // https is fine
		{"https://remote-server", false},
	}
	for _, tc := range cases {
		t.Run(tc.modelAPI, func(t *testing.T) {
			msg := CleartextUpstreamWarning(tc.modelAPI)
			gotWarn := msg != ""
			if gotWarn != tc.wantWarn {
				t.Errorf("CleartextUpstreamWarning(%q): wantWarn=%v, got %q", tc.modelAPI, tc.wantWarn, msg)
			}
			if tc.wantWarn {
				if !strings.Contains(msg, "unencrypted") {
					t.Errorf("warning should mention unencrypted: %q", msg)
				}
				if !strings.Contains(msg, "SSH") && !strings.Contains(msg, "TLS") {
					t.Errorf("warning should suggest SSH tunnel or TLS: %q", msg)
				}
			}
		})
	}
}

func TestConfigFilePath(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{ModelName: "x", ModelAPI: "http://x:8000", Port: 8888}
	if err := cfg.Save(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Errorf("config.json not created: %v", err)
	}
}
