package compiler_test

import (
	"strings"
	"testing"

	"github.com/hollis-labs/loom/internal/compiler"
)

func TestCompileWikiPageWithTemplate(t *testing.T) {
	result, err := compiler.CompileWikiPageWithTemplate(7, `{"title":"Runtime","body":"# Runtime\n\nRunDue notes","source":"notes.md"}`, "# {{title}}\n\nSource: {{source}}\n\n{{body}}")
	if err != nil {
		t.Fatalf("CompileWikiPageWithTemplate: %v", err)
	}
	if result.Page.Title != "Runtime" || result.Page.Slug != "runtime" {
		t.Fatalf("page = %+v", result.Page)
	}
	if strings.Count(result.Page.Body, "# Runtime") != 1 {
		t.Fatalf("duplicate heading in body: %q", result.Page.Body)
	}
	if !strings.Contains(result.Page.Body, "Source: notes.md") || !strings.Contains(result.Page.Body, "RunDue notes") {
		t.Fatalf("body did not render template: %q", result.Page.Body)
	}
}
