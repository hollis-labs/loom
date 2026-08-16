package llm_test

import (
	"strings"
	"testing"

	"github.com/hollis-labs/loom/internal/llm"
)

func TestPromptUserPromptIncludesFields(t *testing.T) {
	p := llm.Prompt{
		Title:       "Runtime",
		Type:        "reference",
		Summary:     "A short summary.",
		Description: "A longer description.",
		Source:      "notes.md",
		Material:    "Raw source notes go here.",
	}
	got := p.UserPrompt()
	for _, want := range []string{
		"Runtime",
		"reference",
		"A short summary.",
		"A longer description.",
		"notes.md",
		"Raw source notes go here.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("UserPrompt() missing %q:\n%s", want, got)
		}
	}
}

func TestPromptUserPromptOmitsBlankOptionalFields(t *testing.T) {
	p := llm.Prompt{Title: "Runtime", Material: "notes"}
	got := p.UserPrompt()
	for _, absent := range []string{"Page type:", "Summary:", "Description:", "Source:"} {
		if strings.Contains(got, absent) {
			t.Fatalf("UserPrompt() unexpectedly contains %q:\n%s", absent, got)
		}
	}
	if !strings.Contains(got, "Page title: Runtime") {
		t.Fatalf("UserPrompt() missing title line:\n%s", got)
	}
	if !strings.Contains(got, "notes") {
		t.Fatalf("UserPrompt() missing material:\n%s", got)
	}
}

func TestSystemPromptDefinesMarkdownOnlyContract(t *testing.T) {
	got := llm.SystemPrompt()
	if strings.TrimSpace(got) == "" {
		t.Fatal("SystemPrompt() is empty")
	}
	if !strings.Contains(got, "Markdown") {
		t.Fatalf("SystemPrompt() missing markdown-output contract:\n%s", got)
	}
}
