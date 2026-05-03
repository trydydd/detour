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

func TestGenerateAuthToken(t *testing.T) {
	t1 := generateAuthToken()
	t2 := generateAuthToken()
	if len(t1) != 64 {
		t.Errorf("token length: want 64, got %d", len(t1))
	}
	if t1 == t2 {
		t.Error("generateAuthToken returned identical values on successive calls")
	}
}
