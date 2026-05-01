package router

import "testing"

func TestLocalAlias(t *testing.T) {
	if got := Route("red", "red"); got != "local" {
		t.Errorf("want local, got %q", got)
	}
}

func TestOpus47(t *testing.T) {
	if got := Route("claude-opus-4-7", "red"); got != "passthrough" {
		t.Errorf("want passthrough, got %q", got)
	}
}

func TestOpus45(t *testing.T) {
	if got := Route("claude-opus-4-5", "red"); got != "passthrough" {
		t.Errorf("want passthrough, got %q", got)
	}
}

func TestOpusLegacy(t *testing.T) {
	if got := Route("claude-3-opus-20240229", "red"); got != "passthrough" {
		t.Errorf("want passthrough, got %q", got)
	}
}

func TestUnknownDefaultsLocal(t *testing.T) {
	if got := Route("some-unknown-model", "red"); got != "local" {
		t.Errorf("want local, got %q", got)
	}
}
