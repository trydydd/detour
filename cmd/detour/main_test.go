package main

import (
	"testing"
)

func TestBindAddress(t *testing.T) {
	want := "127.0.0.1:8888"
	got := buildListenAddr(8888)
	if got != want {
		t.Errorf("buildListenAddr(%d) = %q; want %q", 8888, got, want)
	}
}

