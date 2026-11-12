package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var realHome string

// TestMain points HOME and every XDG directory at a temp directory for the whole package, so
// no test, whatever it forgets to override, can resolve to the real user directories (the
// live installation's data directory is one of them).
func TestMain(m *testing.M) {
	realHome, _ = os.UserHomeDir()
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
	cfg, err := prepare(layout, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cfg, filepath.Join(dir, "identity-signing.key")) || !strings.Contains(cfg, filepath.Join(dir, "secrets")) {
		t.Fatalf("state not beside the db: %s", cfg)
	}
}
