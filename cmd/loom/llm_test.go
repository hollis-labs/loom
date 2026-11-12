package main

import (
	"strings"
	"testing"

	"github.com/hollis-labs/substrate/module"

	"github.com/hollis-labs/loom/internal/service"
	"github.com/hollis-labs/loom/internal/settings"
)

// startApp boots the real modules on a temp data directory with the config module reading the
// given override file (YAML), without serving.
func startApp(t *testing.T, override string) *module.App {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_DATA_HOME", dir+"/data")
	t.Setenv("XDG_STATE_HOME", dir+"/state")
	t.Setenv("XDG_CACHE_HOME", dir+"/cache")
	t.Setenv("XDG_CONFIG_HOME", dir+"/config")
	t.Setenv("SUBSTRATE_CONFIG", "") // the config module reads it from the process environment
	t.Chdir(dir)
	layout, err := resolveLayout(nil)
	if err != nil {
		t.Fatal(err)
	}
	file := ""
	if override != "" {
		file = writeOverride(t, override)
		t.Setenv("SUBSTRATE_CONFIG", file)
	}
	run, err := startModules(t.Context(), layout, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = run.Stop(t.Context()) })
	return run.App
}

func TestConfigureLLMProviderNoopWithoutKeyRef(t *testing.T) {
	app := startApp(t, "")
	comp := service.NewCompiler(nil)
	if err := configureLLMProvider(t.Context(), app, comp, settings.Defaults("")); err != nil {
		t.Fatalf("configureLLMProvider: %v", err)
	}
}

func TestConfigureLLMProviderResolvesKeyRef(t *testing.T) {
	t.Setenv("LOOM_TEST_KEY", "test-key-not-a-real-secret")
	app := startApp(t, "secrets:\n  env: true\n  state_dir: "+t.TempDir()+"\n")
	cfg := settings.Defaults("")
	cfg.LLMAPIKeyRef = "env://LOOM_TEST_KEY"
	cfg.RedactPatterns = []string{`INTERNAL-TOKEN-\d+`}
	// service.Compiler's fields are unexported, so the resulting wiring (redaction actually
	// being applied to LLM-bound content) is asserted end-to-end in internal/service's
	// TestCompileJobLLMModeRedactsConfiguredPatterns.
	if err := configureLLMProvider(t.Context(), app, service.NewCompiler(nil), cfg); err != nil {
		t.Fatalf("configureLLMProvider: %v", err)
	}
}

func TestConfigureLLMProviderRefusesUnresolvableRef(t *testing.T) {
	app := startApp(t, "")
	cfg := settings.Defaults("")
	cfg.LLMAPIKeyRef = "env://LOOM_TEST_KEY_NOT_ALLOWED"
	err := configureLLMProvider(t.Context(), app, service.NewCompiler(nil), cfg)
	if err == nil || !strings.Contains(err.Error(), "loom.llm_api_key_ref") {
		t.Fatalf("err = %v, want one naming loom.llm_api_key_ref", err)
	}
}
