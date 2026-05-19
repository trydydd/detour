package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestBindAddress(t *testing.T) {
	want := "127.0.0.1:8888"
	got := buildListenAddr(8888)
	if got != want {
		t.Errorf("buildListenAddr(%d) = %q; want %q", 8888, got, want)
	}
}

func captureStderr(fn func()) string {
	r, w, _ := os.Pipe()
	old := os.Stderr
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

func TestCheckUpstream_Reachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	out := captureStderr(func() { checkUpstream(srv.URL) })
	want := fmt.Sprintf("detour: upstream %s/v1/models 200 OK\n", srv.URL)
	if out != want {
		t.Errorf("got %q; want %q", out, want)
	}
}

func TestCheckUpstream_Unreachable(t *testing.T) {
	out := captureStderr(func() { checkUpstream("http://127.0.0.1:1") })
	if !bytes.Contains([]byte(out), []byte("warning: model API at http://127.0.0.1:1 is not reachable")) {
		t.Errorf("expected unreachable warning, got %q", out)
	}
}

func TestCheckUpstream_NonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	out := captureStderr(func() { checkUpstream(srv.URL) })
	want := fmt.Sprintf("detour: upstream %s/v1/models 404 Not Found\n", srv.URL)
	if out != want {
		t.Errorf("got %q; want %q", out, want)
	}
}

