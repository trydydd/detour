package router

import "testing"

func TestLocalAlias(t *testing.T) {
	if got := Route("red", "red"); got != "local" {
		t.Errorf("want local, got %q", got)
	}
}

func TestOpusPassthrough(t *testing.T) {
	if got := Route("claude-opus-4-7", "red"); got != "passthrough" {
		t.Errorf("want passthrough, got %q", got)
	}
}

func TestSonnetPassthrough(t *testing.T) {
	if got := Route("claude-sonnet-4-6", "red"); got != "passthrough" {
		t.Errorf("want passthrough, got %q", got)
	}
}

func TestHaikuPassthrough(t *testing.T) {
	if got := Route("claude-haiku-4-5-20251001", "red"); got != "passthrough" {
		t.Errorf("want passthrough, got %q", got)
	}
}

func TestUnknownPassthrough(t *testing.T) {
	if got := Route("some-unknown-model", "red"); got != "passthrough" {
		t.Errorf("want passthrough, got %q", got)
	}
}
