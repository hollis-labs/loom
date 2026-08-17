package compiler_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hollis-labs/loom/internal/compiler"
	"github.com/hollis-labs/loom/internal/llm"
)

// fakeProvider is a minimal in-memory llm.Provider for exercising
// CompileWikiPageWithLLM without any real LLM/network dependency.
type fakeProvider struct {
	text       string
	err        error
	lastPrompt llm.Prompt
	calls      int
}

func (f *fakeProvider) Generate(ctx context.Context, prompt llm.Prompt) (string, error) {
	f.calls++
	f.lastPrompt = prompt
	if f.err != nil {
		return "", f.err
	}
	return f.text, nil
}

func TestCompileWikiPageWithLLMUsesProviderOutputAsBody(t *testing.T) {
	provider := &fakeProvider{text: "# Runtime\n\nLLM-authored body."}
	result, err := compiler.CompileWikiPageWithLLM(context.Background(), provider, 7, `{"title":"Runtime","body":"raw notes","source":"notes.md"}`, "", nil)
	if err != nil {
		t.Fatalf("CompileWikiPageWithLLM: %v", err)
	}
	if result.Page.Title != "Runtime" || result.Page.Slug != "runtime" {
		t.Fatalf("page = %+v", result.Page)
	}
	if result.Page.Body != "# Runtime\n\nLLM-authored body." {
		t.Fatalf("body = %q, want provider output verbatim", result.Page.Body)
	}
	if result.Page.GeneratedBy != compiler.GeneratedByLLM {
		t.Fatalf("GeneratedBy = %q, want %q", result.Page.GeneratedBy, compiler.GeneratedByLLM)
	}
	if result.Page.GeneratedAt == nil {
		t.Fatal("GeneratedAt is nil, want a timestamp")
	}
	if !strings.HasPrefix(result.Document, "---\n") || !strings.Contains(result.Document, result.Page.Body) {
		t.Fatalf("document = %q, want frontmatter + body", result.Document)
	}
	if provider.calls != 1 {
		t.Fatalf("provider called %d times, want 1", provider.calls)
	}
	if provider.lastPrompt.Title != "Runtime" || !strings.Contains(provider.lastPrompt.Material, "raw notes") {
		t.Fatalf("prompt = %+v, want it to carry the request's title/body", provider.lastPrompt)
	}
}

func TestCompileWikiPageWithLLMRendersThroughTemplate(t *testing.T) {
	provider := &fakeProvider{text: "# Runtime\n\nLLM body."}
	result, err := compiler.CompileWikiPageWithLLM(context.Background(), provider, 7, `{"title":"Runtime","body":"raw notes","source":"notes.md"}`, "# {{title}}\n\nSource: {{source}}\n\n{{body}}", nil)
	if err != nil {
		t.Fatalf("CompileWikiPageWithLLM: %v", err)
	}
	if strings.Count(result.Page.Body, "# Runtime") != 1 {
		t.Fatalf("duplicate heading in body: %q", result.Page.Body)
	}
	if !strings.Contains(result.Page.Body, "Source: notes.md") || !strings.Contains(result.Page.Body, "LLM body.") {
		t.Fatalf("body did not render through template: %q", result.Page.Body)
	}
	// The template must render the LLM-generated content, not the raw
	// request body that was sent to the provider as source material.
	if strings.Contains(result.Page.Body, "raw notes") {
		t.Fatalf("body leaked raw source material instead of provider output: %q", result.Page.Body)
	}
}

func TestCompileWikiPageWithLLMRedactsMaterialBeforeSendingToProvider(t *testing.T) {
	provider := &fakeProvider{text: "# Runtime\n\nBody."}
	raw := `{"title":"Runtime","body":"token sk-ant-api03-abcdefghijklmnop is secret"}`
	if _, err := compiler.CompileWikiPageWithLLM(context.Background(), provider, 7, raw, "", nil); err != nil {
		t.Fatalf("CompileWikiPageWithLLM: %v", err)
	}
	if strings.Contains(provider.lastPrompt.Material, "sk-ant-api03") {
		t.Fatalf("prompt material was not redacted: %q", provider.lastPrompt.Material)
	}
	if !strings.Contains(provider.lastPrompt.Material, "[REDACTED]") {
		t.Fatalf("prompt material missing redaction marker: %q", provider.lastPrompt.Material)
	}
}

func TestCompileWikiPageWithLLMRedactsConfiguredPatternsBeforeSendingToProvider(t *testing.T) {
	provider := &fakeProvider{text: "# Runtime\n\nBody."}
	// INTERNAL-TOKEN-<digits> stands in for a company-specific secret
	// format configured via config.Filters.RedactPatterns; none of llm's
	// baseline defaultRedactors would catch it.
	raw := `{"title":"Runtime","body":"token sk-ant-api03-abcdefghijklmnop and INTERNAL-TOKEN-48213 are both secret"}`
	redactPatterns := []string{`INTERNAL-TOKEN-\d+`}
	if _, err := compiler.CompileWikiPageWithLLM(context.Background(), provider, 7, raw, "", redactPatterns); err != nil {
		t.Fatalf("CompileWikiPageWithLLM: %v", err)
	}
	if strings.Contains(provider.lastPrompt.Material, "INTERNAL-TOKEN-48213") {
		t.Fatalf("prompt material was not redacted against the configured pattern: %q", provider.lastPrompt.Material)
	}
	if strings.Contains(provider.lastPrompt.Material, "sk-ant-api03") {
		t.Fatalf("prompt material was not redacted against the baseline pattern: %q", provider.lastPrompt.Material)
	}
}

func TestCompileWikiPageWithLLMNilProviderErrors(t *testing.T) {
	if _, err := compiler.CompileWikiPageWithLLM(context.Background(), nil, 7, `{"title":"Runtime","body":"x"}`, "", nil); err == nil {
		t.Fatal("CompileWikiPageWithLLM: want error for nil provider")
	}
}

func TestCompileWikiPageWithLLMProviderErrorPropagates(t *testing.T) {
	provider := &fakeProvider{err: errors.New("boom")}
	_, err := compiler.CompileWikiPageWithLLM(context.Background(), provider, 7, `{"title":"Runtime","body":"x"}`, "", nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want it to wrap provider error", err)
	}
}

func TestCompileWikiPageWithLLMEmptyResponseErrors(t *testing.T) {
	provider := &fakeProvider{text: "   "}
	if _, err := compiler.CompileWikiPageWithLLM(context.Background(), provider, 7, `{"title":"Runtime","body":"x"}`, "", nil); err == nil {
		t.Fatal("CompileWikiPageWithLLM: want error for blank provider output")
	}
}
