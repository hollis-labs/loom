package compiler_test

import (
	"strings"
	"testing"
	"time"

	"github.com/hollis-labs/loom/internal/compiler"
	"github.com/hollis-labs/loom/internal/domain"
)

func TestCompileWikiPageWithTemplate(t *testing.T) {
	result, err := compiler.CompileWikiPageWithTemplate(7, `{"title":"Runtime","body":"# Runtime\n\nRunDue notes","source":"notes.md"}`, "# {{title}}\n\nSource: {{source}}\n\n{{body}}")
	if err != nil {
		t.Fatalf("CompileWikiPageWithTemplate: %v", err)
	}
	// Regression guard: Body/Title/Slug behavior must be unchanged by the
	// Result.Document addition below.
	if result.Page.Title != "Runtime" || result.Page.Slug != "runtime" {
		t.Fatalf("page = %+v", result.Page)
	}
	if strings.Count(result.Page.Body, "# Runtime") != 1 {
		t.Fatalf("duplicate heading in body: %q", result.Page.Body)
	}
	if !strings.Contains(result.Page.Body, "Source: notes.md") || !strings.Contains(result.Page.Body, "RunDue notes") {
		t.Fatalf("body did not render template: %q", result.Page.Body)
	}
	// Document is a separate frontmatter+body view; it must not change what
	// landed in Page.Body (which feeds Page.ContentHash at persist time).
	if !strings.HasPrefix(result.Document, "---\n") {
		t.Fatalf("document missing frontmatter delimiter: %q", result.Document)
	}
	if !strings.Contains(result.Document, result.Page.Body) {
		t.Fatalf("document does not contain page body verbatim: %q", result.Document)
	}
}

func TestCompileWikiPageWithTemplateAppliesOKFDefaults(t *testing.T) {
	result, err := compiler.CompileWikiPageWithTemplate(7, `{"title":"Runtime","body":"body text","source":"notes.md"}`, "")
	if err != nil {
		t.Fatalf("CompileWikiPageWithTemplate: %v", err)
	}
	// Type fallback: blank Type defaults to "note", mirroring
	// storage.Repository.UpsertPage.
	if result.Page.Type != "note" {
		t.Fatalf("Type = %q, want %q", result.Page.Type, "note")
	}
	if !strings.Contains(result.Document, "type: note") {
		t.Fatalf("document missing defaulted type: %q", result.Document)
	}
	// Sources wrapping: a bare Source wraps into a one-element Sources list.
	if len(result.Page.Sources) != 1 || result.Page.Sources[0] != "notes.md" {
		t.Fatalf("Sources = %+v, want [notes.md]", result.Page.Sources)
	}
	if !strings.Contains(result.Document, "sources:") || !strings.Contains(result.Document, "- notes.md") {
		t.Fatalf("document missing wrapped sources: %q", result.Document)
	}
	// Path/Status also default as part of the same OKF-defaults pass.
	if result.Page.Path != "runtime.md" {
		t.Fatalf("Path = %q, want %q", result.Page.Path, "runtime.md")
	}
	if result.Page.Status != "active" {
		t.Fatalf("Status = %q, want %q", result.Page.Status, "active")
	}
}

func TestRenderFrontmatterFullFieldShape(t *testing.T) {
	stale := time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC)
	generated := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	p := domain.Page{
		Path:              "runtime-notes.md",
		Type:              "reference",
		Title:             "Runtime Notes",
		Description:       "Notes about the runtime.",
		Tags:              []string{"runtime", "notes"},
		Status:            "active",
		StaleAfter:        &stale,
		GeneratedBy:       "loom-compiler",
		GeneratedAt:       &generated,
		Sources:           []string{"notes.md", "other.md"},
		ContentHash:       "abc123",
		SourceFragmentIDs: []int64{1, 2, 3},
	}
	fm := compiler.RenderFrontmatter(p)
	if !strings.HasPrefix(fm, "---\n") || !strings.HasSuffix(fm, "---\n") {
		t.Fatalf("frontmatter missing --- delimiters: %q", fm)
	}
	for _, want := range []string{
		"path: runtime-notes.md",
		"type: reference",
		"title: Runtime Notes",
		"description: Notes about the runtime.",
		"- runtime",
		"- notes",
		"status: active",
		"2026-03-15",
		"generated_by: loom-compiler",
		"2026-08-16T09:00:00Z",
		"- notes.md",
		"- other.md",
		"content_hash: abc123",
		"- 1",
		"- 2",
		"- 3",
	} {
		if !strings.Contains(fm, want) {
			t.Fatalf("frontmatter missing %q:\n%s", want, fm)
		}
	}
	// Family order: core/recommended, lifecycle, trust, provenance.
	idx := func(s string) int { return strings.Index(fm, s) }
	inOrder := idx("path:") < idx("type:") && idx("type:") < idx("title:") &&
		idx("title:") < idx("description:") && idx("description:") < idx("tags:") &&
		idx("tags:") < idx("status:") && idx("status:") < idx("stale_after:") &&
		idx("stale_after:") < idx("generated_by:") && idx("generated_by:") < idx("generated_at:") &&
		idx("generated_at:") < idx("sources:") && idx("sources:") < idx("content_hash:") &&
		idx("content_hash:") < idx("source_fragment_ids:")
	if !inOrder {
		t.Fatalf("frontmatter fields out of family order:\n%s", fm)
	}
}

func TestRenderFrontmatterOptionalFamilyOmission(t *testing.T) {
	fm := compiler.RenderFrontmatter(domain.Page{Type: "note"})
	want := "---\ntype: note\n---\n"
	if fm != want {
		t.Fatalf("frontmatter = %q, want %q", fm, want)
	}
	for _, absent := range []string{
		"path:", "title:", "description:", "tags:", "status:", "stale_after:",
		"generated_by:", "generated_at:", "sources:", "content_hash:", "source_fragment_ids:",
	} {
		if strings.Contains(fm, absent) {
			t.Fatalf("frontmatter unexpectedly contains %q:\n%s", absent, fm)
		}
	}
}

func TestRenderFrontmatterStaleAfterDateTruncation(t *testing.T) {
	stale := time.Date(2026, 3, 15, 14, 30, 45, 0, time.UTC)
	fm := compiler.RenderFrontmatter(domain.Page{Type: "note", StaleAfter: &stale})
	if !strings.Contains(fm, "2026-03-15") {
		t.Fatalf("frontmatter missing truncated stale_after date: %q", fm)
	}
	if strings.Contains(fm, "14:30") {
		t.Fatalf("frontmatter leaked time-of-day into stale_after: %q", fm)
	}
}
