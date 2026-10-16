package server

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Security is who may call loom's API. The zero value is the loopback
// default: no token, loopback Host and browser origins only.
type Security struct {
	// Token, when set, is required as "Authorization: Bearer <token>" on
	// every protected request, loopback included. It is mandatory when serve
	// binds a non-loopback address (see ValidateBind).
	Token string
	// CORSOrigins are browser origins (scheme://host[:port]) allowed besides
	// loopback ones.
	CORSOrigins []string
}

// ValidateBind refuses to expose the API beyond loopback without a token. An
// empty host (":8080") listens on every interface and counts as non-loopback.
func ValidateBind(addr, token string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", addr, err)
	}
	if token == "" && !IsLoopbackHost(host) {
		return fmt.Errorf("refusing to listen on %q without a token: set --token or LOOM_API_TOKEN, or bind to 127.0.0.1", addr)
	}
	return nil
}

// IsLoopbackHost reports whether host is localhost or a loopback IP. An empty
// host is not loopback: it means every interface.
func IsLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// healthPath is the one API route open without a token: container and
// orchestrator probes need it, and it answers only {"status":"ok"}.
const healthPath = "/api/health"

// protectedPath reports whether a request path needs the security policy:
// the JSON API and the HTTP MCP transport. The embedded UI's static files
// do not.
func protectedPath(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/") || path == "/mcp" || strings.HasPrefix(path, "/mcp/")
}

// Protect wraps handler with loom's API security policy.
//
//   - A request carrying a browser Origin that is neither loopback nor
//     configured is refused, in either mode: a cross-site form POST never
//     preflights, so withholding CORS headers alone would still let any web
//     page trigger compiles. Requests without an Origin (CLI, curl,
//     server-to-server) are unaffected.
//   - Without a token the API is loopback-only and a request must name a
//     loopback Host, which refuses DNS rebinding.
//   - With a token every protected request needs the bearer token, except an
//     exact GET/HEAD of /api/health. Preflights pass.
func Protect(handler http.Handler, sec Security) http.Handler {
	want := []byte("Bearer " + sec.Token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !protectedPath(r.URL.Path) {
			handler.ServeHTTP(w, r)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Add("Vary", "Origin")
			if !originAllowed(origin, sec.CORSOrigins) {
				deny(w, http.StatusForbidden, "origin not allowed: set --cors-origin or LOOM_CORS_ORIGINS to allow it")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Mcp-Session-Id, Mcp-Protocol-Version")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if sec.Token == "" {
			if !IsLoopbackHost(requestHostname(r)) {
				deny(w, http.StatusForbidden, "host not allowed: without LOOM_API_TOKEN the API answers only loopback hosts (127.0.0.1, localhost)")
				return
			}
			handler.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == healthPath && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			handler.ServeHTTP(w, r)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="loom"`)
			deny(w, http.StatusUnauthorized, "unauthorized: send Authorization: Bearer <LOOM_API_TOKEN>")
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func originAllowed(origin string, configured []string) bool {
	for _, allowed := range configured {
		if strings.EqualFold(strings.TrimRight(allowed, "/"), origin) {
			return true
		}
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	return IsLoopbackHost(u.Hostname())
}

func requestHostname(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.Host); err == nil {
		return host
	}
	return r.Host
}

func deny(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
