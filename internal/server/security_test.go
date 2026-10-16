package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateBind(t *testing.T) {
	cases := []struct {
		addr, token string
		wantErr     bool
	}{
		{"127.0.0.1:8080", "", false},
		{"[::1]:8080", "", false},
		{"localhost:8080", "", false},
		{":8080", "", true},
		{"0.0.0.0:8080", "", true},
		{"192.168.1.20:8080", "", true},
		{":8080", "t", false},
		{"0.0.0.0:8080", "t", false},
		{"8080", "", true},
	}
	for _, tc := range cases {
		err := ValidateBind(tc.addr, tc.token)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateBind(%q, %q) = %v, wantErr %v", tc.addr, tc.token, err, tc.wantErr)
		}
	}
}

// protectedServer serves every path with 200 "ok" behind Protect, recording
// whether the inner handler ran.
func protectedServer(t *testing.T, sec Security) *httptest.Server {
	t.Helper()
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})
	srv := httptest.NewServer(Protect(inner, sec))
	t.Cleanup(srv.Close)
	return srv
}

func call(t *testing.T, method, url string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestTokenRequiredOnAPIAndMCP(t *testing.T) {
	srv := protectedServer(t, Security{Token: "s3cret"})
	for _, path := range []string{"/api/bundles", "/api/status", "/api", "/mcp", "/mcp/x"} {
		for name, tc := range map[string]struct {
			auth string
			want int
		}{
			"missing":      {"", http.StatusUnauthorized},
			"wrong":        {"Bearer nope", http.StatusUnauthorized},
			"wrong scheme": {"Basic s3cret", http.StatusUnauthorized},
			"correct":      {"Bearer s3cret", http.StatusOK},
		} {
			headers := map[string]string{}
			if tc.auth != "" {
				headers["Authorization"] = tc.auth
			}
			resp := call(t, http.MethodPost, srv.URL+path, headers)
			if resp.StatusCode != tc.want {
				t.Errorf("%s %s: status %d, want %d", path, name, resp.StatusCode, tc.want)
			}
			if tc.want == http.StatusUnauthorized && !strings.Contains(resp.Header.Get("WWW-Authenticate"), "Bearer") {
				t.Errorf("%s %s: missing WWW-Authenticate", path, name)
			}
		}
	}
}

// Health is open without a token, but only at the exact path and only for
// reads; a longer path or a write still needs the token.
func TestHealthExemptionIsExact(t *testing.T) {
	srv := protectedServer(t, Security{Token: "s3cret"})
	if resp := call(t, http.MethodGet, srv.URL+"/api/health", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/health = %d, want 200", resp.StatusCode)
	}
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/health/x"},
		{http.MethodGet, "/api/healthz"},
		{http.MethodPost, "/api/health"},
	} {
		if resp := call(t, tc.method, srv.URL+tc.path, nil); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", tc.method, tc.path, resp.StatusCode)
		}
	}
}

func TestUIIsNotGated(t *testing.T) {
	srv := protectedServer(t, Security{Token: "s3cret"})
	for _, path := range []string{"/", "/assets/app.js", "/bundles/nanite"} {
		if resp := call(t, http.MethodGet, srv.URL+path, nil); resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, resp.StatusCode)
		}
	}
}

func TestTokenlessIsLoopbackOnly(t *testing.T) {
	srv := protectedServer(t, Security{})
	if resp := call(t, http.MethodPost, srv.URL+"/api/ingest/text", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("tokenless loopback = %d, want 200", resp.StatusCode)
	}
	if resp := call(t, http.MethodGet, srv.URL+"/api/bundles", map[string]string{"Host": "rebind.example:8080"}); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("tokenless rebinding Host = %d, want 403", resp.StatusCode)
	}
}

// Requests without an Origin (CLI, curl, server-to-server) are never
// refused for lacking one; a present foreign Origin always is.
func TestOriginPolicy(t *testing.T) {
	for name, sec := range map[string]Security{
		"tokenless":  {CORSOrigins: []string{"https://ui.example"}},
		"with token": {Token: "s3cret", CORSOrigins: []string{"https://ui.example"}},
	} {
		t.Run(name, func(t *testing.T) {
			srv := protectedServer(t, sec)
			auth := map[string]string{}
			if sec.Token != "" {
				auth["Authorization"] = "Bearer s3cret"
			}
			with := func(origin string) map[string]string {
				h := map[string]string{}
				for k, v := range auth {
					h[k] = v
				}
				if origin != "" {
					h["Origin"] = origin
				}
				return h
			}
			if resp := call(t, http.MethodPost, srv.URL+"/api/ingest/text", with("")); resp.StatusCode != http.StatusOK {
				t.Fatalf("no Origin = %d, want 200", resp.StatusCode)
			}
			for origin, allowed := range map[string]bool{
				"http://localhost:5173":         true,
				"http://127.0.0.1:8092":         true,
				"https://ui.example":            true,
				"https://evil.example":          false,
				"http://localhost.evil.example": false,
				"null":                          false,
			} {
				resp := call(t, http.MethodPost, srv.URL+"/api/ingest/text", with(origin))
				got := resp.Header.Get("Access-Control-Allow-Origin")
				if allowed && (resp.StatusCode != http.StatusOK || got != origin) {
					t.Errorf("origin %s: status %d, ACAO %q; want 200 and echoed", origin, resp.StatusCode, got)
				}
				if !allowed && (resp.StatusCode != http.StatusForbidden || got != "") {
					t.Errorf("origin %s: status %d, ACAO %q; want 403 and none", origin, resp.StatusCode, got)
				}
			}
			preflight := call(t, http.MethodOptions, srv.URL+"/api/bundles", map[string]string{"Origin": "http://localhost:5173", "Access-Control-Request-Method": "POST"})
			if preflight.StatusCode != http.StatusNoContent || preflight.Header.Get("Access-Control-Allow-Origin") == "*" {
				t.Errorf("preflight = %d, ACAO %q", preflight.StatusCode, preflight.Header.Get("Access-Control-Allow-Origin"))
			}
		})
	}
}
