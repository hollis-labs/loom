// Package settings reads the app's own "loom" block from the manifest config. The shell modules
// read theirs; this block holds what only Loom has.
//
//	loom:
//	  export_dir: ""                 # where exports are written; default <data>/exports
//	  redact_patterns: []            # regexps redacted from ingested text and from LLM prompts
//	  git_exclude_paths: [.git, node_modules, dist]   # path segments the git/file ingest skips
//	  llm_model: claude-sonnet-4-6   # model used for generation_mode=llm compile jobs
//	  llm_api_key_ref: ""            # secret reference to the Anthropic API key; empty = LLM off
//
// The key reference is resolved by the secrets module (keychain://, file://, op://, helper://, or
// env://NAME when secrets.env is true). The key itself is never in config, a response or a log.
package settings

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Key is the name of the config block.
const Key = "loom"

// DefaultLLMModel is used when llm_model is empty.
const DefaultLLMModel = "claude-sonnet-4-6"

// Settings are the parsed block.
type Settings struct {
	ExportDir      string
	AllowedHosts   []string
	RedactPatterns []string
	GitExclude     []string
	LLMModel       string
	LLMAPIKeyRef   string
	OTelEnabled    bool
	OTelMetrics    bool
	OTelEndpoint   string
}

// Defaults are the settings of an empty block. exportDir is <data>/exports, which only the
// caller knows.
func Defaults(exportDir string) Settings {
	return Settings{
		ExportDir:  exportDir,
		GitExclude: []string{".git", "node_modules", "dist"},
		LLMModel:   DefaultLLMModel,
	}
}

// Parse reads the block (nil for a missing one) over the defaults. An unknown key or a bad value
// is an error naming the key, so a typo is found at startup instead of silently turning a
// protection off.
func Parse(raw any, exportDir string) (Settings, error) {
	s := Defaults(exportDir)
	if raw == nil {
		return s, nil
	}
	block, ok := raw.(map[string]any)
	if !ok {
		return s, fmt.Errorf("loom: the config block must be a map")
	}
	known := map[string]bool{"export_dir": true, "allowed_hosts": true, "redact_patterns": true, "git_exclude_paths": true, "llm_model": true, "llm_api_key_ref": true, "otel_enabled": true, "otel_metrics": true, "otel_endpoint": true}
	var unknown []string
	for k := range block {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return s, fmt.Errorf("loom: unknown config key(s): %s", strings.Join(unknown, ", "))
	}
	str := func(k string, dst *string) error {
		v, present := block[k]
		if !present || v == nil {
			return nil
		}
		t, ok := v.(string)
		if !ok {
			return fmt.Errorf("loom.%s must be a string", k)
		}
		if t = strings.TrimSpace(t); t != "" {
			*dst = t
		}
		return nil
	}
	boolean := func(k string, dst *bool) error {
		v, present := block[k]
		if !present || v == nil {
			return nil
		}
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf("loom.%s must be true or false", k)
		}
		*dst = b
		return nil
	}
	list := func(k string, dst *[]string) error {
		v, present := block[k]
		if !present || v == nil {
			return nil
		}
		items, ok := v.([]any)
		if !ok {
			return fmt.Errorf("loom.%s must be a list of strings", k)
		}
		out := make([]string, 0, len(items))
		for _, it := range items {
			t, ok := it.(string)
			if !ok {
				return fmt.Errorf("loom.%s must be a list of strings", k)
			}
			out = append(out, t)
		}
		*dst = out
		return nil
	}
	for _, step := range []error{
		str("export_dir", &s.ExportDir),
		str("llm_model", &s.LLMModel),
		str("llm_api_key_ref", &s.LLMAPIKeyRef),
		str("otel_endpoint", &s.OTelEndpoint),
		boolean("otel_enabled", &s.OTelEnabled),
		boolean("otel_metrics", &s.OTelMetrics),
		list("redact_patterns", &s.RedactPatterns),
		list("git_exclude_paths", &s.GitExclude),
		list("allowed_hosts", &s.AllowedHosts),
	} {
		if step != nil {
			return s, step
		}
	}
	for _, p := range s.RedactPatterns {
		if _, err := regexp.Compile(p); err != nil {
			return s, fmt.Errorf("loom.redact_patterns: %q is not a valid regular expression: %w", p, err)
		}
	}
	return s, nil
}
