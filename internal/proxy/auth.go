package proxy

import (
	"net/http"
	"os"
)

// authMiddleware checks for a valid X-Detour-Auth header.
// The expected token is read from ANTHROPIC_DETOUR_AUTH env var.
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	expectedToken := os.Getenv("ANTHROPIC_DETOUR_AUTH")

	return func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health endpoint
		if r.URL.Path == "/health" {
			next(w, r)
			return
		}

		if expectedToken == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "proxy auth not configured")
			return
		}

		token := r.Header.Get("X-Detour-Auth")
		if token != expectedToken {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing auth token")
			return
		}

		next(w, r)
	}
}
