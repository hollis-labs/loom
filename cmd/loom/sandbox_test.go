package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hollis-labs/substrate/module"
)

var realHome string

// realGoEnv is GOMODCACHE and GOCACHE as they were before the sandbox changed HOME: the tests that
// run `go build` use them, so the build neither downloads modules into the sandbox (a read-only
// tree the cleanup cannot remove) nor starts from a cold cache.
var realGoEnv []string

// TestMain points HOME and every XDG directory at a temp directory for the whole package, so
// no test, whatever it forgets to override, can resolve to the real user directories (the
// live installation's data directory is one of them).
func TestMain(m *testing.M) {
	realHome, _ = os.UserHomeDir()
	for _, k := range []string{"GOMODCACHE", "GOCACHE"} {
		if out, err := exec.Command("go", "env", k).Output(); err == nil { //nolint:gosec // fixed arguments
			realGoEnv = append(realGoEnv, k+"="+strings.TrimSpace(string(out)))
		}
	}
	dir, err := os.MkdirTemp("", "loom-test-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for k, v := range map[string]string{
		"HOME": dir, "XDG_DATA_HOME": filepath.Join(dir, "data"), "XDG_STATE_HOME": filepath.Join(dir, "state"),
		"XDG_CACHE_HOME": filepath.Join(dir, "cache"), "XDG_CONFIG_HOME": filepath.Join(dir, "config"),
		"SUBSTRATE_CONFIG": "",
	} {
		os.Setenv(k, v) //nolint:errcheck // a failed Setenv leaves the sandbox check below to fail
	}
	code := m.Run()
	removeBuilt()
	os.RemoveAll(dir) //nolint:errcheck,gosec // best-effort temp cleanup
	os.Exit(code)
}

func TestDefaultLayoutIsSandboxed(t *testing.T) {
	layout, err := resolveLayout([]string{"-no-project"})
	if err != nil {
		t.Fatal(err)
	}
	if realHome != "" && strings.HasPrefix(layout.DataDir(), filepath.Join(realHome, ".local")) {
		t.Fatalf("default data dir %s resolves to the real XDG location", layout.DataDir())
	}
	if !strings.Contains(layout.DataDir(), "loom-test-home-") {
		t.Fatalf("default data dir %s is not in the test sandbox", layout.DataDir())
	}
}

// An explicit -db keeps the identity key and secrets beside that file, not in the XDG default.
func TestExplicitDBKeepsStateBesideIt(t *testing.T) {
	dir := t.TempDir()
	layout, err := resolveLayout([]string{"-no-project", "-db", filepath.Join(dir, "x.db")})
	if err != nil {
		t.Fatal(err)
	}
	app := &module.App{Config: map[string]any{
		"database": map[string]any{"sqlite": map[string]any{"path": "loom.db"}},
		"identity": map[string]any{"key_file": "identity-signing.key"},
		"secrets":  map[string]any{"state_dir": "secrets"},
	}}
	resolvePaths(app, layout, true)
	cfg := fmt.Sprint(app.Config)
	for _, want := range []string{filepath.Join(dir, "x.db"), filepath.Join(dir, "identity-signing.key"), filepath.Join(dir, "secrets")} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("%s not in %s", want, cfg)
		}
	}
}

// A path the operator's override made absolute is left where they put it.
func TestResolvePathsKeepsAbsolute(t *testing.T) {
	layout, err := resolveLayout([]string{"-no-project"})
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(t.TempDir(), "elsewhere.db")
	app := &module.App{Config: map[string]any{"database": map[string]any{"sqlite": map[string]any{"path": abs}}}}
	resolvePaths(app, layout, false)
	if got := app.Config["database"].(map[string]any)["sqlite"].(map[string]any)["path"]; got != abs {
		t.Fatalf("path = %v, want %s", got, abs)
	}
}
