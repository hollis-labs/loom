package config

import (
	"os"
	"path/filepath"
	"testing"

	paths "github.com/hollis-labs/libs/util/apppaths"
)

func TestLoadMergesUserThenProjectAndExpandsPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LOOM_TEST_ROOT", filepath.Join(home, "root"))
	t.Chdir(t.TempDir())
	layout, err := paths.Resolve("loom", paths.WithProjectMode())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := os.MkdirAll(layout.ConfigDir(), 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	userFile := filepath.Join(layout.ConfigDir(), FileName)
	if err := os.WriteFile(userFile, []byte(`
paths:
  corpus_dir: ~/corpus
drafts:
  provider: claude
filters:
  redact_patterns: ["secret"]
`), 0o644); err != nil {
		t.Fatalf("write user config: %v", err)
	}
	projectFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(projectFile, []byte(`
paths:
  export_dir: ${LOOM_TEST_ROOT}/exports
drafts:
  model: claude-sonnet
sources:
  git:
    - name: loom
      path: ~/dev/loom
`), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	cfg, loaded, err := Load(layout, projectFile)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded = %+v, want user and project", loaded)
	}
	if cfg.Paths.CorpusDir != filepath.Join(home, "corpus") {
		t.Fatalf("corpus_dir = %q", cfg.Paths.CorpusDir)
	}
	if cfg.Paths.ExportDir != filepath.Join(home, "root", "exports") {
		t.Fatalf("export_dir = %q", cfg.Paths.ExportDir)
	}
	if cfg.Drafts.Provider != "claude" || cfg.Drafts.Model != "claude-sonnet" {
		t.Fatalf("drafts = %+v", cfg.Drafts)
	}
	if len(cfg.Sources.Git) != 1 || cfg.Sources.Git[0].Path != filepath.Join(home, "dev", "loom") {
		t.Fatalf("git sources = %+v", cfg.Sources.Git)
	}
}

func TestLoadFileMissingReturnsNotExist(t *testing.T) {
	_, err := LoadFile(filepath.Join(t.TempDir(), "missing.yaml"))
	if !os.IsNotExist(err) {
		t.Fatalf("err = %v, want os.ErrNotExist", err)
	}
}
