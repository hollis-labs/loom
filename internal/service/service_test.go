package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hollis-labs/loom/internal/compiler"
	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/service"
	"github.com/hollis-labs/loom/internal/storage"
)

func TestCompileJobCompletesAndStoresPage(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := storage.NewRepository(db)
	compiler := service.NewCompiler(repo)

	job, err := compiler.Request(ctx, "nanite", "wiki_page", "# Scheduler\n\nRunDue notes")
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if job.Status != "completed" {
		t.Fatalf("status = %q, want completed: %+v", job.Status, job)
	}
	page, err := repo.GetPageBySlug(ctx, "nanite", "scheduler")
	if err != nil {
		t.Fatalf("GetPageBySlug: %v", err)
	}
	if page.Title != "Scheduler" || page.Body == "" {
		t.Fatalf("page = %+v", page)
	}
	if strings.Count(page.Body, "# Scheduler") != 1 {
		t.Fatalf("body has duplicate title heading: %q", page.Body)
	}
}

func TestCompileUsesStoredTemplate(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := storage.NewRepository(db)
	if _, err := repo.UpsertTemplate(ctx, domain.Template{Name: "wiki_page.sourceful", Generator: "wiki_page", Body: "# {{title}}\n\n_Source: {{source}}_\n\n{{body}}"}); err != nil {
		t.Fatalf("UpsertTemplate: %v", err)
	}
	input, err := json.Marshal(compiler.Request{Title: "Templated", Body: "Body text", Source: "test-source", Template: "wiki_page.sourceful"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	job, err := service.NewCompiler(repo).Request(ctx, "nanite", "wiki_page", string(input))
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if job.Status != "completed" {
		t.Fatalf("job = %+v", job)
	}
	page, err := repo.GetPageBySlug(ctx, "nanite", "templated")
	if err != nil {
		t.Fatalf("GetPageBySlug: %v", err)
	}
	if !strings.Contains(page.Body, "_Source: test-source_") || !strings.Contains(page.Body, "Body text") {
		t.Fatalf("page body = %q", page.Body)
	}
}

func TestCompileFailureStoresFailedJobAndEvents(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := storage.NewRepository(db)
	input, err := json.Marshal(compiler.Request{Title: "Missing Template", Body: "Body", Template: "wiki_page.missing"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	job, err := service.NewCompiler(repo).Request(ctx, "nanite", "wiki_page", string(input))
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if job.Status != "failed" || !strings.Contains(job.Error, "load template wiki_page.missing") {
		t.Fatalf("job = %+v, want failed missing-template job", job)
	}
	events, err := repo.ListEvents(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 3 || events[0].Status != "queued" || events[1].Status != "running" || events[2].Status != "failed" {
		t.Fatalf("events = %+v", events)
	}
	stats, err := repo.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.FailedJobs != 1 || stats.CompletedJobs != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	if _, err := repo.GetPageBySlug(ctx, "nanite", "missing-template"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("GetPageBySlug err = %v, want ErrNotFound", err)
	}
}

func TestRequestFromDirectivesIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := storage.NewRepository(db)
	compiler := service.NewCompiler(repo)
	text := "::config generator=wiki_page, slug=directive-page\n::note Directive Page"

	first, err := compiler.RequestFromDirectives(ctx, "nanite", text, "test")
	if err != nil {
		t.Fatalf("first RequestFromDirectives: %v", err)
	}
	second, err := compiler.RequestFromDirectives(ctx, "nanite", text, "test")
	if err != nil {
		t.Fatalf("second RequestFromDirectives: %v", err)
	}
	if len(first.Jobs) != 1 {
		t.Fatalf("first jobs = %+v, want one", first.Jobs)
	}
	if len(second.Jobs) != 0 || len(second.Skipped) != 1 {
		t.Fatalf("second result = %+v, want duplicate skipped", second)
	}
	page, err := repo.GetPageBySlug(ctx, "nanite", "directive-page")
	if err != nil {
		t.Fatalf("GetPageBySlug: %v", err)
	}
	if page.Title != "Directive Page" {
		t.Fatalf("page title = %q", page.Title)
	}
}

func TestIngestInvalidInputsUseStorageInvalid(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	compiler := service.NewCompiler(storage.NewRepository(db))

	if _, err := compiler.IngestFiles(ctx, "nanite", nil, "fixture", nil, nil); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("IngestFiles err = %v, want storage.ErrInvalid", err)
	}
	if _, err := compiler.IngestText(ctx, "nanite", "text", "", "", "", nil); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("IngestText err = %v, want storage.ErrInvalid", err)
	}
}

func TestIngestFilesIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "runtime.md")
	if err := os.WriteFile(path, []byte("# Runtime\n\nToken abc123"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	repo := storage.NewRepository(db)
	compiler := service.NewCompiler(repo)

	first, err := compiler.IngestFiles(ctx, "nanite", []string{dir}, "fixture", nil, []string{`abc\d+`})
	if err != nil {
		t.Fatalf("first IngestFiles: %v", err)
	}
	second, err := compiler.IngestFiles(ctx, "nanite", []string{dir}, "fixture", nil, []string{`abc\d+`})
	if err != nil {
		t.Fatalf("second IngestFiles: %v", err)
	}
	if first.Items != 1 || len(first.Jobs) != 1 || len(first.Skipped) != 0 {
		t.Fatalf("first result = %+v", first)
	}
	if second.Items != 1 || len(second.Jobs) != 0 || len(second.Skipped) != 1 {
		t.Fatalf("second result = %+v", second)
	}
	page, err := repo.GetPageBySlug(ctx, "nanite", "runtime")
	if err != nil {
		t.Fatalf("GetPageBySlug: %v", err)
	}
	if page.Title != "Runtime" || page.Source != path {
		t.Fatalf("page = %+v", page)
	}
	if page.Body == "" || page.Body == "# Runtime\n\nToken abc123" {
		t.Fatalf("page body was not redacted: %q", page.Body)
	}
}

func TestCompileConfidenceNewPage(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := storage.NewRepository(db)
	comp := service.NewCompiler(repo)

	input, err := json.Marshal(compiler.Request{Title: "Confidence New", Slug: "confidence-new", Body: "# Confidence New\n\nFirst line.\nSecond line.\n"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	job, err := comp.Request(ctx, "nanite", "wiki_page", string(input))
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if job.Status != "completed" {
		t.Fatalf("job = %+v", job)
	}
	var output domain.CompileOutput
	if err := json.Unmarshal([]byte(job.Output), &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if output.PageExisted {
		t.Fatalf("output.PageExisted = true, want false for a brand-new page: %+v", output)
	}
	if output.Confidence != 1.0 || output.ConfidenceTier != "high" {
		t.Fatalf("output confidence = %v/%q, want 1.0/high: %+v", output.Confidence, output.ConfidenceTier, output)
	}
	if output.Diff != nil {
		t.Fatalf("output.Diff = %+v, want nil for a brand-new page (nothing to compare against)", output.Diff)
	}
}

func TestCompileConfidenceIdenticalOverwrite(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := storage.NewRepository(db)
	comp := service.NewCompiler(repo)
	body := "# Confidence Overwrite\n\nSame line one.\nSame line two.\n"
	input, err := json.Marshal(compiler.Request{Title: "Confidence Overwrite", Slug: "confidence-overwrite", Body: body})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := comp.Request(ctx, "nanite", "wiki_page", string(input)); err != nil {
		t.Fatalf("first Request: %v", err)
	}

	job, err := comp.Request(ctx, "nanite", "wiki_page", string(input))
	if err != nil {
		t.Fatalf("second Request: %v", err)
	}
	if job.Status != "completed" {
		t.Fatalf("job = %+v", job)
	}
	var output domain.CompileOutput
	if err := json.Unmarshal([]byte(job.Output), &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if !output.PageExisted {
		t.Fatalf("output.PageExisted = false, want true: %+v", output)
	}
	if output.Confidence != 1.0 || output.ConfidenceTier != "high" {
		t.Fatalf("output confidence = %v/%q, want 1.0/high for a no-op overwrite: %+v", output.Confidence, output.ConfidenceTier, output)
	}
	if output.Diff == nil {
		t.Fatalf("output.Diff = nil, want a trivial (no-op) diff summary")
	}
	if output.Diff.OldContentHash != output.Diff.NewContentHash {
		t.Fatalf("diff hashes = %q/%q, want equal for an identical-body overwrite", output.Diff.OldContentHash, output.Diff.NewContentHash)
	}
	if output.Diff.LineDelta != 0 || output.Diff.SimilarityRatio != 1.0 || output.Diff.UnifiedDiff != "" {
		t.Fatalf("diff = %+v, want zero line delta, similarity 1.0, empty unified diff", output.Diff)
	}
}

func TestCompileConfidenceSubstantialOverwrite(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := storage.NewRepository(db)
	comp := service.NewCompiler(repo)

	firstBody := "# Doc\n\nAlpha line.\nBeta line.\nGamma line.\nDelta line.\nEpsilon line."
	firstInput, err := json.Marshal(compiler.Request{Title: "Doc", Slug: "confidence-rewrite", Body: firstBody})
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	if _, err := comp.Request(ctx, "nanite", "wiki_page", string(firstInput)); err != nil {
		t.Fatalf("first Request: %v", err)
	}

	secondBody := "# Doc\n\nZulu different line.\nYankee other line.\nXray unrelated line.\nWhiskey new line.\nVictor extra line.\nUniform more line.\nTango final line."
	secondInput, err := json.Marshal(compiler.Request{Title: "Doc", Slug: "confidence-rewrite", Body: secondBody})
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	job, err := comp.Request(ctx, "nanite", "wiki_page", string(secondInput))
	if err != nil {
		t.Fatalf("second Request: %v", err)
	}
	if job.Status != "completed" {
		t.Fatalf("job = %+v", job)
	}
	var output domain.CompileOutput
	if err := json.Unmarshal([]byte(job.Output), &output); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if !output.PageExisted {
		t.Fatalf("output.PageExisted = false, want true: %+v", output)
	}
	if output.Diff == nil {
		t.Fatalf("output.Diff = nil, want a real diff summary for a substantially different overwrite")
	}
	if output.Diff.OldContentHash == output.Diff.NewContentHash {
		t.Fatalf("diff hashes are equal, want different content hashes for a substantially different overwrite")
	}
	if output.Diff.LineDelta != 2 {
		t.Fatalf("diff.LineDelta = %d, want 2 (7 old lines -> 9 new lines): %+v", output.Diff.LineDelta, output.Diff)
	}
	if output.Diff.SimilarityRatio >= 0.5 {
		t.Fatalf("diff.SimilarityRatio = %v, want < 0.5 for near-entirely-different content", output.Diff.SimilarityRatio)
	}
	if output.Confidence != output.Diff.SimilarityRatio || output.ConfidenceTier != "low" {
		t.Fatalf("output confidence = %v/%q, want similarity-ratio/low: %+v", output.Confidence, output.ConfidenceTier, output)
	}
	if !strings.Contains(output.Diff.UnifiedDiff, "-Alpha line.") || !strings.Contains(output.Diff.UnifiedDiff, "+Zulu different line.") {
		t.Fatalf("unified diff missing expected -/+ lines: %q", output.Diff.UnifiedDiff)
	}
}

func TestIngestTextIsIdempotent(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := storage.NewRepository(db)
	compiler := service.NewCompiler(repo)

	first, err := compiler.IngestText(ctx, "nanite", "chatgpt", "thread-1", "Thread Notes", "Body token abc123", []string{`abc\d+`})
	if err != nil {
		t.Fatalf("first IngestText: %v", err)
	}
	second, err := compiler.IngestText(ctx, "nanite", "chatgpt", "thread-1", "Thread Notes", "Body token abc123", []string{`abc\d+`})
	if err != nil {
		t.Fatalf("second IngestText: %v", err)
	}
	if len(first.Jobs) != 1 || len(first.Skipped) != 0 {
		t.Fatalf("first = %+v", first)
	}
	if len(second.Jobs) != 0 || len(second.Skipped) != 1 {
		t.Fatalf("second = %+v", second)
	}
	page, err := repo.GetPageBySlug(ctx, "nanite", "thread-notes")
	if err != nil {
		t.Fatalf("GetPageBySlug: %v", err)
	}
	if !strings.Contains(page.Body, "[REDACTED]") || strings.Contains(page.Body, "abc123") {
		t.Fatalf("page body was not redacted: %q", page.Body)
	}
}
