package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()
	if cfg.Port != 8888 {
		t.Errorf("default port: want 8888, got %d", cfg.Port)
	}
	if cfg.AlsoSonnet != false {
		t.Error("default AlsoSonnet should be false")
	}
}

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	orig := &Config{
		Port:       9000,
		ModelName:  "red",
		ModelAPI:   "http://192.168.0.28",
		AlsoSonnet: true,
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
	saved := &Config{Port: 9000, ModelName: "saved", ModelAPI: "http://saved"}
	if err := saved.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Simulate flags arriving with a different port and model name.
	flags := &Config{Port: 7777, ModelName: "flags", ModelAPI: "http://flags"}
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
		{"valid", Config{Port: 8888, ModelName: "red", ModelAPI: "http://x"}, true},
		{"missing ModelName", Config{Port: 8888, ModelAPI: "http://x"}, false},
		{"missing ModelAPI", Config{Port: 8888, ModelName: "red"}, false},
		{"missing both", Config{Port: 8888}, false},
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

func TestConfigFilePath(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{ModelName: "x", ModelAPI: "http://x", Port: 8888}
	if err := cfg.Save(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Errorf("config.json not created: %v", err)
	}
}
