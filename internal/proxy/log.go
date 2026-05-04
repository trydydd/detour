package proxy

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// maybeLog wraps next with logRequests only when DETOUR_LOG is truthy.
// Accepted truthy values: 1, true, yes, on (case-insensitive).
func maybeLog(next http.HandlerFunc) http.HandlerFunc {
	if !logEnabled(os.Getenv("DETOUR_LOG")) {
		return next
	}
	return logRequests(next)
}

func logEnabled(v string) bool {
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// logRequests wraps a handler to write one stderr line per request:
// "detour: <method> <path> <status> <duration>".
func logRequests(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingWriter{ResponseWriter: w, status: 200}
		next(lw, r)
		fmt.Fprintf(os.Stderr, "detour: %s %s %d %s\n",
			r.Method, r.URL.RequestURI(), lw.status, time.Since(start).Round(time.Millisecond))
	}
}

type loggingWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (l *loggingWriter) WriteHeader(code int) {
	if !l.wrote {
		l.status = code
		l.wrote = true
	}
	l.ResponseWriter.WriteHeader(code)
}

func (l *loggingWriter) Write(b []byte) (int, error) {
	l.wrote = true
	return l.ResponseWriter.Write(b)
}

func (l *loggingWriter) Flush() {
	if f, ok := l.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
