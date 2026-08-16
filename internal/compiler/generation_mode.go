package compiler

import (
	"encoding/json"
	"strings"
)

// GenerationMode selects which compiler path produces a wiki page's body.
type GenerationMode string

const (
	// GenerationModeDeterministic is Loom's default template-based compiler
	// (CompileWikiPageWithTemplate): no LLM calls, fully offline and
	// reproducible.
	GenerationModeDeterministic GenerationMode = "deterministic"
	// GenerationModeLLM delegates body authoring to a configured LLM
	// Provider (internal/llm, see CompileWikiPageWithLLM), then renders
	// the result through the same template pipeline as the deterministic
	// path.
	GenerationModeLLM GenerationMode = "llm"
)

// Normalized returns the effective mode, defaulting empty or unrecognized
// values to GenerationModeDeterministic so LLM generation is always
// explicit opt-in.
func (m GenerationMode) Normalized() GenerationMode {
	if m == GenerationModeLLM {
		return GenerationModeLLM
	}
	return GenerationModeDeterministic
}

// RequestGenerationMode extracts Request.GenerationMode from a raw compile
// job input, mirroring TemplateName's tolerant JSON parsing: non-JSON or
// unparseable input is treated as GenerationModeDeterministic rather than
// an error, since raw plain-text input (no JSON envelope) is a normal,
// supported Request shape.
func RequestGenerationMode(raw string) GenerationMode {
	if strings.TrimSpace(raw) == "" || !json.Valid([]byte(raw)) {
		return GenerationModeDeterministic
	}
	var req Request
	_ = json.Unmarshal([]byte(raw), &req)
	return req.GenerationMode.Normalized()
}
