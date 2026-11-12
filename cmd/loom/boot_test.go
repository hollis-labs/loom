package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hollis-labs/substrate/boot"
)

func writeOverride(t *testing.T, yaml string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "override.yaml")
	if err := os.WriteFile(file, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	return file
}

// started boots the real stack (every module the manifest selects, the real migrations, the
// real handler chain) on a temp data directory and a free loopback port, and returns its base
// URL. It is the acceptance test: nothing is stubbed between the HTTP client and the database.
func started(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("SUBSTRATE_CONFIG", "") // the config module reads it from the process environment
	t.Chdir(dir)
	layout, err := resolveLayout(nil)
	if err != nil {
		t.Fatal(err)
	}
	config, err := prepare(layout, "", false)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err = json.Unmarshal([]byte(config), &cfg); err != nil {
		t.Fatal(err)
	}
	cfg["httphost"] = map[string]any{"addr": "127.0.0.1:0"} // port 0 asks for a free one
	config2, _ := json.Marshal(cfg)

	useMigrations()
	ctx, cancel := context.WithCancel(context.Background())
	run, err := boot.Start(ctx, substrateApp, substrateModules, string(config2))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	addr := make(chan net.Addr, 1)
	l, err := newLoom(ctx, run.App, layout)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	l.ready = func(a net.Addr) { addr <- a }
	done := make(chan error, 1)
	go func() { done <- l.serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("serve ended with %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Error("serve did not stop on cancel")
		}
		_ = run.Stop(context.Background())
	})
	select {
	case a := <-addr:
		return "http://" + a.String()
	case err := <-done:
		t.Fatalf("serve returned before listening: %v", err)
	case <-time.After(15 * time.Second):
		t.Fatal("serve never listened")
	}
	return ""
}

type session struct {
	t    *testing.T
	base string
	http *http.Client
	xrw  bool // sends the X-Requested-With header the identity module requires of cookie writes
}

func newSession(t *testing.T, base string) *session {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &session{t: t, base: base, http: &http.Client{Jar: jar, Timeout: 10 * time.Second}, xrw: true}
}

func (s *session) do(method, path string, body any, headers map[string]string) (int, map[string]any) {
	s.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			s.t.Fatal(err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, s.base+path, rdr)
	if err != nil {
		s.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.xrw {
		req.Header.Set("X-Requested-With", "loom-ui")
	}
	for k, v := range headers {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	res, err := s.http.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return res.StatusCode, out
}

func (s *session) want(status int, method, path string, body any) map[string]any {
	s.t.Helper()
	got, out := s.do(method, path, body, nil)
	if got != status {
		s.t.Fatalf("%s %s = %d %v, want %d", method, path, got, out, status)
	}
	return out
}

// The binary serves health without credentials, refuses the API to an anonymous caller, lets the
// first-run owner set up and sign in, and then serves reads and writes with the flat error body.
func TestServeSetupSignInWriteReadRefuse(t *testing.T) {
	base := started(t)
	s := newSession(t, base)

	s.want(http.StatusOK, "GET", "/healthz", nil)
	out := s.want(http.StatusUnauthorized, "GET", "/api/status", nil)
	if out["code"] == nil || out["message"] == nil {
		t.Fatalf("unauthenticated error is not the flat body: %v", out)
	}
	s.want(http.StatusOK, "GET", "/api/setup", nil)
	s.want(http.StatusOK, "POST", "/api/setup", map[string]any{"display_name": "Owner", "username": "owner", "password": "correct horse battery staple"})

	// A write: a bundle. A read: it comes back. A refused request: the same write without the
	// X-Requested-With header a cookie-authenticated browser must send.
	s.want(http.StatusOK, "POST", "/api/bundles", map[string]any{"slug": "docs", "title": "Docs"})
	got := s.want(http.StatusOK, "GET", "/api/bundles/docs", nil)
	if got["slug"] != "docs" {
		t.Fatalf("bundle = %v", got)
	}
	s.xrw = false
	s.want(http.StatusForbidden, "POST", "/api/bundles", map[string]any{"slug": "nope", "title": "Nope"})
	s.xrw = true

	// An unknown route is refused by the guard (fail closed), not routed.
	s.want(http.StatusForbidden, "GET", "/api/undeclared", nil)
	// An unknown Host is refused before any handler (DNS rebinding).
	if code, out := s.do("GET", "/api/bundles", nil, map[string]string{"Host": "evil.example"}); code != http.StatusForbidden || out["code"] == nil {
		t.Fatalf("unknown Host = %d %v, want 403 with the flat body", code, out)
	}
}

// `loom migrate-data` copies a pre-Substrate workspace database next to it, startup refuses to run
// on the old data until that has been done, and the migrated file opens with the real modules.
func TestMigrateDataThenStart(t *testing.T) {
	fixtures, err := filepath.Abs("../../internal/storage/testdata/legacy")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(dir, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("SUBSTRATE_CONFIG", "")
	t.Chdir(dir)
	layout, err := resolveLayout([]string{"-project=false"})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(layout.MainDB()), 0o700); err != nil {
		t.Fatal(err)
	}
	// An old-layout file with one synthetic bundle.
	old, err := sql.Open("sqlite", layout.MainDB())
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(fixtures, "*.sql"))
	if _, err = old.Exec(`CREATE TABLE schema_version (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		body, _ := os.ReadFile(f) //nolint:gosec // test fixture path
		if _, err = old.Exec(string(body)); err != nil {
			t.Fatal(err)
		}
		if _, err = old.Exec(`INSERT INTO schema_version (version) VALUES (?)`, strings.TrimSuffix(filepath.Base(f), ".sql")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = old.Exec(`INSERT INTO wiki_bundles (id, slug, title) VALUES (5,'kept','Kept')`); err != nil {
		t.Fatal(err)
	}
	_ = old.Close()

	if _, err = prepare(layout, "", false); err == nil || !strings.Contains(err.Error(), "migrate-data") {
		t.Fatalf("start on unmigrated data: err = %v, want a refusal naming migrate-data", err)
	}
	var out bytes.Buffer
	if err = migrateData(context.Background(), layout, false, nil, &out); err != nil {
		t.Fatalf("migrate-data: %v", err)
	}
	if !strings.Contains(out.String(), `"wiki_bundles":1`) {
		t.Fatalf("report = %s", out.String())
	}
	if err = migrateData(context.Background(), layout, false, nil, &out); err == nil {
		t.Fatal("second migrate-data run accepted")
	}
	config, err := prepare(layout, "", false)
	if err != nil {
		t.Fatalf("start after migration: %v", err)
	}
	useMigrations()
	run, err := boot.Start(t.Context(), substrateApp, substrateModules, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = run.Stop(t.Context()) }()
	l, err := newLoom(t.Context(), run.App, layout)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := l.repo.GetBundle(t.Context(), "kept"); err != nil || b.ID != 5 {
		t.Fatalf("migrated bundle = %+v %v", b, err)
	}
}

// /mcp needs loom.mcp, and each tool needs its own scope on top: a read key lists and reads but
// cannot call a write tool.
func TestMCPToolsNeedTheirScopes(t *testing.T) {
	base := started(t)
	s := newSession(t, base)
	s.want(http.StatusOK, "POST", "/api/setup", map[string]any{"display_name": "Owner", "username": "owner", "password": "correct horse battery staple"})
	key := func(scopes ...string) string {
		out := s.want(http.StatusCreated, "POST", "/api/identity/keys", map[string]any{"name": "k", "scopes": scopes})
		k, _ := out["key"].(string)
		if k == "" {
			t.Fatalf("no key in %v", out)
		}
		return k
	}
	call := func(k, tool string, args map[string]any) (int, string) {
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}})
		req, _ := http.NewRequest(http.MethodPost, base+"/mcp", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+k)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		raw, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(raw)
	}
	readKey := key("loom.mcp", "loom.read")
	if code, out := call(readKey, "loom_status", map[string]any{}); code != http.StatusOK || strings.Contains(out, `"isError":true`) {
		t.Fatalf("read tool with a read key = %d %s", code, out)
	}
	if _, out := call(readKey, "loom_bundle_put", map[string]any{"slug": "x", "title": "X"}); !strings.Contains(out, `"isError":true`) {
		t.Fatalf("write tool with a read key succeeded: %s", out)
	}
	if code, _ := call(key("loom.read", "loom.write"), "loom_status", map[string]any{}); code != http.StatusForbidden {
		t.Fatalf("a key without loom.mcp reached /mcp: %d", code)
	}
	if _, out := call(key("loom.mcp", "loom.read", "loom.write"), "loom_bundle_put", map[string]any{"slug": "x", "title": "X"}); strings.Contains(out, "isError") {
		t.Fatalf("write tool with a write key failed: %s", out)
	}
}
