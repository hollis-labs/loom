// Package llm decouples Loom's compiler from any specific LLM SDK. Concrete
// providers live in subpackages (e.g. internal/llm/anthropic); callers in
// internal/compiler and internal/service depend only on the Provider
// interface defined here.
package llm

import "context"

// Provider generates a wiki page body (markdown) from a Prompt. Implementations
// call out to an LLM API; internal/compiler.CompileWikiPageWithLLM treats the
// returned string as the page's markdown body, then runs it through the same
// template-rendering pipeline as the deterministic compiler.
type Provider interface {
	// Generate returns markdown for the given prompt, or an error if the
	// underlying LLM call fails or returns no usable content.
	Generate(ctx context.Context, prompt Prompt) (string, error)
}
