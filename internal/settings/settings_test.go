package settings

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseDefaults(t *testing.T) {
	s, err := Parse(nil, "/data/exports")
	if err != nil {
		t.Fatal(err)
	}
	want := Settings{ExportDir: "/data/exports", GitExclude: []string{".git", "node_modules", "dist"}, LLMModel: DefaultLLMModel}
	if !reflect.DeepEqual(s, want) {
		t.Fatalf("defaults = %+v, want %+v", s, want)
	}
}

func TestParseOverrides(t *testing.T) {
	s, err := Parse(map[string]any{
		"export_dir":        "/elsewhere",
		"redact_patterns":   []any{`secret=\w+`},
		"git_exclude_paths": []any{"vendor"},
		"llm_model":         "m",
		"llm_api_key_ref":   "keychain://loom/anthropic",
		"otel_enabled":      true,
		"otel_endpoint":     "127.0.0.1:4318",
	}, "/data/exports")
	if err != nil {
		t.Fatal(err)
	}
	if s.ExportDir != "/elsewhere" || s.LLMModel != "m" || s.LLMAPIKeyRef != "keychain://loom/anthropic" || !s.OTelEnabled || s.OTelMetrics || s.OTelEndpoint != "127.0.0.1:4318" {
		t.Fatalf("settings = %+v", s)
	}
	if !reflect.DeepEqual(s.GitExclude, []string{"vendor"}) || !reflect.DeepEqual(s.RedactPatterns, []string{`secret=\w+`}) {
		t.Fatalf("lists = %+v", s)
	}
}

func TestParseRefusesBadInput(t *testing.T) {
	for name, tc := range map[string]struct {
		raw  any
		want string
	}{
		"not a map":     {"x", "must be a map"},
		"unknown key":   {map[string]any{"export_dr": "/x"}, "export_dr"},
		"wrong type":    {map[string]any{"llm_model": 3}, "loom.llm_model"},
		"wrong list":    {map[string]any{"redact_patterns": "a"}, "loom.redact_patterns"},
		"wrong bool":    {map[string]any{"otel_enabled": "yes"}, "loom.otel_enabled"},
		"bad regexp":    {map[string]any{"redact_patterns": []any{"("}}, "regular expression"},
		"bad list item": {map[string]any{"git_exclude_paths": []any{1}}, "loom.git_exclude_paths"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(tc.raw, "/x")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}
