package config

import (
	"os"
	"path/filepath"
	"testing"

	paths "github.com/hollis-labs/libs/util/apppaths"
)

func TestLoadMergesLLMSection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())
	layout, err := paths.Resolve("loom", paths.WithProjectMode())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	projectFile := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(projectFile, []byte(`
llm:
  provider: anthropic
  model: claude-sonnet-4-6
`), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	cfg, _, err := Load(layout, projectFile)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LLM.Provider != "anthropic" || cfg.LLM.Model != "claude-sonnet-4-6" {
		t.Fatalf("llm = %+v, want provider/model from project config", cfg.LLM)
	}
}

func TestDefaultLeavesLLMConfigBlank(t *testing.T) {
	cfg := Default(mustLayout(t))
	if cfg.LLM.Provider != "" || cfg.LLM.Model != "" {
		t.Fatalf("default llm = %+v, want zero value (opt-in only)", cfg.LLM)
	}
}

func mustLayout(t *testing.T) paths.Layout {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())
	layout, err := paths.Resolve("loom", paths.WithProjectMode())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return layout
}
