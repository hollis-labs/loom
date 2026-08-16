package service_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/service"
	"github.com/hollis-labs/loom/internal/storage"
)

// newDirectiveTestRepo returns a fresh, migrated in-memory-backed repository
// with the seeded "nanite" bundle (see storage.Migrate), matching the setup
// every other service_test.go test uses.
func newDirectiveTestRepo(t *testing.T) *storage.Repository {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return storage.NewRepository(db)
}

// jobPage decodes the domain.Page a completed compile job persisted, from
// its stored Output JSON (see Compiler.run: `{"page": <domain.Page>}`).
// Reading the job's own recorded output (rather than re-deriving the page's
// slug and calling GetPageBySlug) keeps these tests independent of any
// particular slugging scheme.
func jobPage(t *testing.T, job domain.CompileJob) domain.Page {
	t.Helper()
	if job.Status != "completed" {
		t.Fatalf("job status = %q, want completed: %+v", job.Status, job)
	}
	var out struct {
		Page domain.Page `json:"page"`
	}
	if err := json.Unmarshal([]byte(job.Output), &out); err != nil {
		t.Fatalf("unmarshal job output: %v (output=%q)", err, job.Output)
	}
	return out.Page
}

// TestDirectiveDispatchRoutesEachCommandToItsHandler is the dispatch-routing
// test: one directive per Command (draft, log-adr, reminder, extract) is
// parsed and compiled through RequestFromDirectives, and each is asserted
// to have reached its own handler — observable via the OKF Type each
// handler sets on the resulting page (draft leaves Type unset, which
// applyOKFDefaults then defaults to "note"; the other three set a distinct
// Type each). If dispatch silently fell through to the generic handler for
// any of these commands, Type would come back "" (defaulted to "note")
// instead of the command-specific value.
func TestDirectiveDispatchRoutesEachCommandToItsHandler(t *testing.T) {
	cases := []struct {
		command  string
		text     string
		wantType string
	}{
		{"draft", "::draft Runtime Notes\nSome body content.", "note"},
		{"log-adr", "::log-adr Use SQLite for storage.", "adr"},
		{"reminder", "::reminder Renew the domain by Friday.", "reminder"},
		{"extract", "Intro line.\n::extract Grab this.\nTrailing line.", "extract"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			repo := newDirectiveTestRepo(t)
			compiler := service.NewCompiler(repo)
			result, err := compiler.RequestFromDirectives(context.Background(), "nanite", tc.text, "test")
			if err != nil {
				t.Fatalf("RequestFromDirectives: %v", err)
			}
			if len(result.Jobs) != 1 {
				t.Fatalf("jobs = %+v, want exactly one", result.Jobs)
			}
			if len(result.Failed) != 0 {
				t.Fatalf("failed = %+v, want none", result.Failed)
			}
			page := jobPage(t, result.Jobs[0])
			if page.Type != tc.wantType {
				t.Fatalf("command %s: page.Type = %q, want %q (page=%+v)", tc.command, page.Type, tc.wantType, page)
			}
		})
	}
}

// TestDraftDirectiveProducesPageEndToEnd exercises the draft handler
// end-to-end: parse -> ledger -> dispatch -> compile -> persist, the same
// pipeline "loom directives parse" feeds into via
// directivex.ParseAndLedger/RequestFromDirectives. It also confirms draft
// reuses the *full* source text as page body (unlike extract, see
// TestExtractDirectiveUsesContextRangeNotFullText) and that it's retrievable
// through the normal page-lookup path, not just the job's own output.
func TestDraftDirectiveProducesPageEndToEnd(t *testing.T) {
	repo := newDirectiveTestRepo(t)
	compiler := service.NewCompiler(repo)
	text := "Intro line.\n::draft Runtime Notes\nTrailing detail line."

	result, err := compiler.RequestFromDirectives(context.Background(), "nanite", text, "test")
	if err != nil {
		t.Fatalf("RequestFromDirectives: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("jobs = %+v, want exactly one", result.Jobs)
	}
	job := result.Jobs[0]
	if job.Status != "completed" {
		t.Fatalf("job status = %q: %+v", job.Status, job)
	}

	page, err := repo.GetPageBySlug(context.Background(), "nanite", "runtime-notes")
	if err != nil {
		t.Fatalf("GetPageBySlug: %v", err)
	}
	if page.Title != "Runtime Notes" {
		t.Fatalf("page title = %q, want %q", page.Title, "Runtime Notes")
	}
	if page.Type != "note" {
		t.Fatalf("page type = %q, want default %q", page.Type, "note")
	}
	if !strings.Contains(page.Body, "Intro line.") || !strings.Contains(page.Body, "Trailing detail line.") {
		t.Fatalf("draft page body should contain the full source text, got: %q", page.Body)
	}

	// Re-parsing the identical text is idempotent: the directive hash is
	// already ledgered, so it's Skipped rather than re-dispatched.
	second, err := compiler.RequestFromDirectives(context.Background(), "nanite", text, "test")
	if err != nil {
		t.Fatalf("second RequestFromDirectives: %v", err)
	}
	if len(second.Jobs) != 0 || len(second.Skipped) != 1 {
		t.Fatalf("second result = %+v, want one skipped duplicate", second)
	}
}

// TestLogADRDirectiveTagsPageAsADR checks handleLogADRDirective's specific
// judgment calls: OKF Type "adr", an "ADR: " title prefix (not doubled when
// the author already wrote one), and body sourced from the directive's own
// prompt rather than the full surrounding text.
func TestLogADRDirectiveTagsPageAsADR(t *testing.T) {
	repo := newDirectiveTestRepo(t)
	compiler := service.NewCompiler(repo)
	text := "Some unrelated preamble.\n::log-adr Use SQLite for storage because it is simple."

	result, err := compiler.RequestFromDirectives(context.Background(), "nanite", text, "test")
	if err != nil {
		t.Fatalf("RequestFromDirectives: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("jobs = %+v, want exactly one", result.Jobs)
	}
	page := jobPage(t, result.Jobs[0])
	if page.Type != "adr" {
		t.Fatalf("page type = %q, want %q", page.Type, "adr")
	}
	if !strings.HasPrefix(page.Title, "ADR: ") {
		t.Fatalf("page title = %q, want ADR: prefix", page.Title)
	}
	if strings.Contains(page.Body, "Some unrelated preamble.") {
		t.Fatalf("log-adr body should be the directive prompt, not the surrounding text: %q", page.Body)
	}
	if !strings.Contains(page.Body, "Use SQLite for storage") {
		t.Fatalf("log-adr body missing the ADR text: %q", page.Body)
	}

	// An author-supplied "ADR" prefix is not doubled.
	repo2 := newDirectiveTestRepo(t)
	result2, err := service.NewCompiler(repo2).RequestFromDirectives(context.Background(), "nanite", "::log-adr ADR-002: Prefer SQLite.", "test")
	if err != nil {
		t.Fatalf("RequestFromDirectives: %v", err)
	}
	page2 := jobPage(t, result2.Jobs[0])
	if strings.Count(strings.ToUpper(page2.Title), "ADR") != 1 {
		t.Fatalf("page title = %q, want a single ADR marker (no doubled prefix)", page2.Title)
	}
}

// TestReminderDirectiveIsLightweight checks handleReminderDirective's
// judgment call: the body is just the directive's own prompt (no LLM pass,
// no surrounding-context or template expansion), and an empty prompt still
// produces a valid page instead of dumping the raw compile-request JSON.
func TestReminderDirectiveIsLightweight(t *testing.T) {
	repo := newDirectiveTestRepo(t)
	compiler := service.NewCompiler(repo)

	result, err := compiler.RequestFromDirectives(context.Background(), "nanite", "::reminder Renew the domain by Friday.", "test")
	if err != nil {
		t.Fatalf("RequestFromDirectives: %v", err)
	}
	page := jobPage(t, result.Jobs[0])
	if page.Type != "reminder" {
		t.Fatalf("page type = %q, want %q", page.Type, "reminder")
	}
	if !strings.HasPrefix(page.Title, "Reminder: ") {
		t.Fatalf("page title = %q, want Reminder: prefix", page.Title)
	}
	if !strings.Contains(page.Body, "Renew the domain by Friday.") {
		t.Fatalf("reminder body missing prompt text: %q", page.Body)
	}

	repo2 := newDirectiveTestRepo(t)
	resultBare, err := service.NewCompiler(repo2).RequestFromDirectives(context.Background(), "nanite", "::reminder", "test")
	if err != nil {
		t.Fatalf("RequestFromDirectives (bare): %v", err)
	}
	if len(resultBare.Jobs) != 1 || resultBare.Jobs[0].Status != "completed" {
		t.Fatalf("bare reminder result = %+v, want one completed job", resultBare)
	}
	barePage := jobPage(t, resultBare.Jobs[0])
	if strings.Contains(barePage.Body, "{") {
		t.Fatalf("bare reminder body looks like raw JSON, not a rendered page: %q", barePage.Body)
	}
}

// TestExtractDirectiveUsesContextRangeNotFullText checks
// handleExtractDirective's judgment call: the page body is the source lines
// go-directives attributed to the directive (Directive.ContextRange), which
// excludes trailing content after the directive's own line — unlike draft,
// which always uses the full source text (see
// TestDraftDirectiveProducesPageEndToEnd).
func TestExtractDirectiveUsesContextRangeNotFullText(t *testing.T) {
	repo := newDirectiveTestRepo(t)
	compiler := service.NewCompiler(repo)
	text := "Intro line before the directive.\n::extract Grab this.\nTrailing line after the directive."

	result, err := compiler.RequestFromDirectives(context.Background(), "nanite", text, "test")
	if err != nil {
		t.Fatalf("RequestFromDirectives: %v", err)
	}
	page := jobPage(t, result.Jobs[0])
	if page.Type != "extract" {
		t.Fatalf("page type = %q, want %q", page.Type, "extract")
	}
	if !strings.Contains(page.Body, "Intro line before the directive.") {
		t.Fatalf("extract body missing context line: %q", page.Body)
	}
	if strings.Contains(page.Body, "Trailing line after the directive.") {
		t.Fatalf("extract body should not include content after the directive's own line: %q", page.Body)
	}
}

// TestRequestFromDirectivesRecordsUnsupportedGeneratorAsFailure is the
// "not silently dropped" regression test for CW-20260816-0017: before this
// task, a directive whose ::config generator=... override wasn't
// "wiki_page" was quietly appended to Skipped, indistinguishable from an
// idempotent duplicate. It must now show up in DirectiveCompileResult.Failed
// with a clear reason, and must not abort the rest of the batch.
func TestRequestFromDirectivesRecordsUnsupportedGeneratorAsFailure(t *testing.T) {
	repo := newDirectiveTestRepo(t)
	compiler := service.NewCompiler(repo)
	// ::config cascades to every directive in its scope, so the bad
	// generator override is scoped to a context_start/context_end block
	// around just the draft directive — otherwise it would also cascade
	// to (and fail) the log-adr directive below, defeating the point of
	// this test (proving one directive's failure doesn't sink the batch).
	text := "::context_start Scratch\n::config generator=mystery_generator\n::draft Won't compile\n::context_end\n::log-adr Will compile fine."

	result, err := compiler.RequestFromDirectives(context.Background(), "nanite", text, "test")
	if err != nil {
		t.Fatalf("RequestFromDirectives: %v", err)
	}
	if len(result.Failed) != 1 {
		t.Fatalf("failed = %+v, want exactly one failure", result.Failed)
	}
	failure := result.Failed[0]
	if failure.Command != "draft" || !strings.Contains(failure.Reason, "generator") {
		t.Fatalf("failure = %+v, want a draft-command generator failure", failure)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("jobs = %+v, want the log-adr directive to still complete", result.Jobs)
	}
}
