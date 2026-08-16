package compiler_test

import (
	"testing"

	"github.com/hollis-labs/loom/internal/compiler"
)

func TestGenerationModeNormalized(t *testing.T) {
	cases := []struct {
		mode compiler.GenerationMode
		want compiler.GenerationMode
	}{
		{"", compiler.GenerationModeDeterministic},
		{"deterministic", compiler.GenerationModeDeterministic},
		{"llm", compiler.GenerationModeLLM},
		{"bogus", compiler.GenerationModeDeterministic},
	}
	for _, tc := range cases {
		if got := tc.mode.Normalized(); got != tc.want {
			t.Fatalf("GenerationMode(%q).Normalized() = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

func TestRequestGenerationMode(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want compiler.GenerationMode
	}{
		{"blank", "", compiler.GenerationModeDeterministic},
		{"plain text", "# Just a heading\n\nbody text", compiler.GenerationModeDeterministic},
		{"json without mode", `{"title":"Runtime"}`, compiler.GenerationModeDeterministic},
		{"json deterministic", `{"title":"Runtime","generation_mode":"deterministic"}`, compiler.GenerationModeDeterministic},
		{"json llm", `{"title":"Runtime","generation_mode":"llm"}`, compiler.GenerationModeLLM},
		{"json unknown mode", `{"title":"Runtime","generation_mode":"bogus"}`, compiler.GenerationModeDeterministic},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := compiler.RequestGenerationMode(tc.raw); got != tc.want {
				t.Fatalf("RequestGenerationMode(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}
