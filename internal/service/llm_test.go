package service_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hollis-labs/loom/internal/compiler"
	"github.com/hollis-labs/loom/internal/llm"
	"github.com/hollis-labs/loom/internal/service"
	"github.com/hollis-labs/loom/internal/storage"
)

// fakeProvider is a minimal in-memory llm.Provider, kept local to this test
// file (internal/compiler has its own copy) so service tests don't need to
// export a test double from another package.
type fakeProvider struct {
	text       string
	lastPrompt llm.Prompt
}

func (f *fakeProvider) Generate(ctx context.Context, prompt llm.Prompt) (string, error) {
	f.lastPrompt = prompt
	return f.text, nil
}

func TestCompileJobUsesLLMProviderWhenGenerationModeIsLLM(t *testing.T) {
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
	comp.SetLLMProvider(&fakeProvider{text: "# Scheduler\n\nLLM-authored body."}, nil)

	input, err := json.Marshal(compiler.Request{Title: "Scheduler", Body: "raw notes", GenerationMode: compiler.GenerationModeLLM})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	job, err := comp.Request(ctx, "nanite", "wiki_page", string(input))
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
	if page.Body != "# Scheduler\n\nLLM-authored body." {
		t.Fatalf("page body = %q, want LLM output verbatim", page.Body)
	}
	if page.GeneratedBy != compiler.GeneratedByLLM {
		t.Fatalf("GeneratedBy = %q, want %q", page.GeneratedBy, compiler.GeneratedByLLM)
	}
	events, err := repo.ListEvents(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	var sawLLMStart bool
	for _, ev := range events {
		if ev.Status == "running" && strings.Contains(ev.Message, "llm") {
			sawLLMStart = true
		}
	}
	if !sawLLMStart {
		t.Fatalf("events = %+v, want a running event mentioning the llm compiler", events)
	}
}

// TestCompileJobLLMModeRedactsConfiguredPatterns proves the redactPatterns
// passed to SetLLMProvider (typically config.Filters.RedactPatterns) flow
// all the way through Compiler.Request -> run -> CompileWikiPageWithLLM and
// are applied to the job body before it reaches the LLM Provider, alongside
// llm's baseline defaultRedactors.
func TestCompileJobLLMModeRedactsConfiguredPatterns(t *testing.T) {
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
	provider := &fakeProvider{text: "# Scheduler\n\nLLM-authored body."}
	// INTERNAL-TOKEN-<digits> stands in for a company-specific secret
	// format a user would configure via config.Filters.RedactPatterns;
	// none of llm's baseline defaultRedactors match this shape.
	comp.SetLLMProvider(provider, []string{`INTERNAL-TOKEN-\d+`})

	input, err := json.Marshal(compiler.Request{
		Title:          "Scheduler",
		Body:           "token sk-ant-api03-abcdefghijklmnop and INTERNAL-TOKEN-77120 are both secret",
		GenerationMode: compiler.GenerationModeLLM,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	job, err := comp.Request(ctx, "nanite", "wiki_page", string(input))
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if job.Status != "completed" {
		t.Fatalf("status = %q, want completed: %+v", job.Status, job)
	}
	if strings.Contains(provider.lastPrompt.Material, "INTERNAL-TOKEN-77120") {
		t.Fatalf("prompt material was not redacted against the configured pattern: %q", provider.lastPrompt.Material)
	}
	if strings.Contains(provider.lastPrompt.Material, "sk-ant-api03") {
		t.Fatalf("prompt material was not redacted against the baseline pattern: %q", provider.lastPrompt.Material)
	}
}

func TestCompileJobLLMModeWithoutProviderFails(t *testing.T) {
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

	input, err := json.Marshal(compiler.Request{Title: "Scheduler", Body: "raw notes", GenerationMode: compiler.GenerationModeLLM})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	job, err := comp.Request(ctx, "nanite", "wiki_page", string(input))
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if job.Status != "failed" {
		t.Fatalf("status = %q, want failed: %+v", job.Status, job)
	}
	if !strings.Contains(job.Error, "provider") {
		t.Fatalf("job.Error = %q, want it to mention the missing provider", job.Error)
	}
}

func TestCompileJobDefaultGenerationModeStillDeterministic(t *testing.T) {
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
	// No LLM provider configured; a job with no generation_mode (or
	// "deterministic") must still succeed via the deterministic compiler.
	job, err := comp.Request(ctx, "nanite", "wiki_page", "# Scheduler\n\nRunDue notes")
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
	if page.GeneratedBy != compiler.GeneratedBy {
		t.Fatalf("GeneratedBy = %q, want %q", page.GeneratedBy, compiler.GeneratedBy)
	}
}
