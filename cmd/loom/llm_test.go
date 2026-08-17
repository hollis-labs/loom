package main

import (
	"os"
	"path/filepath"
	"testing"

	paths "github.com/hollis-labs/go-apppaths/paths"
	"github.com/hollis-labs/loom/internal/service"
	"github.com/hollis-labs/loom/internal/storage"
)

func testLayout(t *testing.T) paths.Layout {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Chdir(t.TempDir())
	layout, err := resolveLayout(nil)
	if err != nil {
		t.Fatalf("resolveLayout: %v", err)
	}
	return layout
}

func newTestCompiler(t *testing.T) *service.Compiler {
	t.Helper()
	db, err := storage.Open(t.Context(), filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return service.NewCompiler(storage.NewRepository(db))
}

func TestConfigureLLMProviderNoopWithoutAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	layout := testLayout(t)
	comp := newTestCompiler(t)
	if err := configureLLMProvider(comp, layout); err != nil {
		t.Fatalf("configureLLMProvider: %v", err)
	}
	// No direct way to observe an unset provider from outside the package;
	// the meaningful assertion is that this does not error and does not
	// require ANTHROPIC_API_KEY to be a valid credential.
}

func TestConfigureLLMProviderUsesConfiguredModel(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key-not-a-real-secret")
	layout := testLayout(t)
	if err := os.WriteFile(filepath.Join(layout.ConfigDir(), "config.yaml"), []byte("llm:\n  model: claude-sonnet-4-6\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	comp := newTestCompiler(t)
	if err := configureLLMProvider(comp, layout); err != nil {
		t.Fatalf("configureLLMProvider: %v", err)
	}
}

func TestConfigureLLMProviderPassesFilterRedactPatterns(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key-not-a-real-secret")
	layout := testLayout(t)
	if err := os.WriteFile(filepath.Join(layout.ConfigDir(), "config.yaml"), []byte("filters:\n  redact_patterns:\n    - \"INTERNAL-TOKEN-\\\\d+\"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	comp := newTestCompiler(t)
	// configureLLMProvider must load cfg.Filters.RedactPatterns and pass it
	// through to comp.SetLLMProvider without erroring; service.Compiler's
	// fields are unexported so the resulting wiring (redaction actually
	// being applied to LLM-bound content) is asserted end-to-end in
	// internal/service's TestCompileJobLLMModeRedactsConfiguredPatterns.
	if err := configureLLMProvider(comp, layout); err != nil {
		t.Fatalf("configureLLMProvider: %v", err)
	}
}

func TestConfigureLLMProviderFallsBackToDefaultModel(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key-not-a-real-secret")
	layout := testLayout(t)
	comp := newTestCompiler(t)
	// No config.yaml at all: cfg.LLM.Model is blank, so configureLLMProvider
	// must fall back to defaultLLMModel instead of erroring.
	if err := configureLLMProvider(comp, layout); err != nil {
		t.Fatalf("configureLLMProvider: %v", err)
	}
}
