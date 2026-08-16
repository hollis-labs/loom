package storage_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/storage"
)

func openRepo(t *testing.T) *storage.Repository {
	t.Helper()
	db, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storage.Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := storage.Migrate(context.Background(), db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	return storage.NewRepository(db)
}

func TestMigrateSeedsNaniteIdempotently(t *testing.T) {
	repo := openRepo(t)
	applied, err := storage.AppliedMigrations(context.Background(), repo.DB())
	if err != nil {
		t.Fatalf("AppliedMigrations: %v", err)
	}
	wantApplied := []string{"001_wiki_core", "002_compile_jobs", "003_directives_templates", "004_ingest_ledger", "005_default_template_body"}
	if len(applied) != len(wantApplied) {
		t.Fatalf("applied migrations = %+v, want %+v", applied, wantApplied)
	}
	for i := range wantApplied {
		if applied[i] != wantApplied[i] {
			t.Fatalf("applied migrations = %+v, want %+v", applied, wantApplied)
		}
	}
	bundles, err := repo.ListBundles(context.Background())
	if err != nil {
		t.Fatalf("ListBundles: %v", err)
	}
	if len(bundles) != 1 || bundles[0].Slug != "nanite" {
		t.Fatalf("bundles = %+v, want one nanite bundle", bundles)
	}
	tpl, err := repo.GetTemplate(context.Background(), "wiki_page.default")
	if err != nil {
		t.Fatalf("GetTemplate: %v", err)
	}
	if tpl.Body != "# {{title}}\n\n{{body}}" {
		t.Fatalf("default template body = %q", tpl.Body)
	}
}

func TestRepositoryListMethodsReturnEmptySlices(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	b, err := repo.GetBundle(ctx, "nanite")
	if err != nil {
		t.Fatalf("GetBundle: %v", err)
	}
	pages, err := repo.ListPages(ctx, b.ID, "missing", 10)
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if pages == nil || len(pages) != 0 {
		t.Fatalf("pages = %+v, want empty non-nil slice", pages)
	}
	links, err := repo.ListPageLinks(ctx, -1)
	if err != nil {
		t.Fatalf("ListPageLinks: %v", err)
	}
	if links == nil || len(links) != 0 {
		t.Fatalf("links = %+v, want empty non-nil slice", links)
	}
	verifications, err := repo.ListPageVerifications(ctx, -1)
	if err != nil {
		t.Fatalf("ListPageVerifications: %v", err)
	}
	if verifications == nil || len(verifications) != 0 {
		t.Fatalf("verifications = %+v, want empty non-nil slice", verifications)
	}
	jobs, err := repo.ListJobs(ctx, storage.JobFilter{BundleID: b.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if jobs == nil || len(jobs) != 0 {
		t.Fatalf("jobs = %+v, want empty non-nil slice", jobs)
	}
	directives, err := repo.ListDirectives(ctx, 10)
	if err != nil {
		t.Fatalf("ListDirectives: %v", err)
	}
	if directives == nil || len(directives) != 0 {
		t.Fatalf("directives = %+v, want empty non-nil slice", directives)
	}
	ingests, err := repo.ListIngest(ctx, 10)
	if err != nil {
		t.Fatalf("ListIngests: %v", err)
	}
	if ingests == nil || len(ingests) != 0 {
		t.Fatalf("ingests = %+v, want empty non-nil slice", ingests)
	}
}

func TestRepositoryInvalidInputsUseSentinel(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	b, err := repo.GetBundle(ctx, "nanite")
	if err != nil {
		t.Fatalf("GetBundle: %v", err)
	}
	if _, err := repo.UpsertBundle(ctx, domain.Bundle{}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("UpsertBundle err = %v, want ErrInvalid", err)
	}
	if _, err := repo.UpsertPage(ctx, domain.Page{BundleID: b.ID}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("UpsertPage err = %v, want ErrInvalid", err)
	}
	if _, err := repo.UpsertTemplate(ctx, domain.Template{Name: "bad"}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("UpsertTemplate err = %v, want ErrInvalid", err)
	}
}

func TestPageCRUDAndSearch(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	b, err := repo.GetBundle(ctx, "nanite")
	if err != nil {
		t.Fatalf("GetBundle: %v", err)
	}
	page, err := repo.UpsertPage(ctx, domain.Page{BundleID: b.ID, Slug: "runtime-notes", Title: "Runtime Notes", Summary: "scheduler", Body: "# Runtime Notes"})
	if err != nil {
		t.Fatalf("UpsertPage: %v", err)
	}
	got, err := repo.GetPageBySlug(ctx, "nanite", "runtime-notes")
	if err != nil {
		t.Fatalf("GetPageBySlug: %v", err)
	}
	if got.ID != page.ID || got.Title != "Runtime Notes" {
		t.Fatalf("got %+v, want %+v", got, page)
	}
	results, err := repo.ListPages(ctx, b.ID, "scheduler", 10)
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if len(results) != 1 || results[0].Slug != "runtime-notes" {
		t.Fatalf("search results = %+v", results)
	}
	normalized, err := repo.UpsertPage(ctx, domain.Page{BundleID: b.ID, Slug: "Release Notes 2026", Title: "Release Notes", Body: "# Release Notes"})
	if err != nil {
		t.Fatalf("normalized UpsertPage: %v", err)
	}
	if normalized.Slug != "release-notes-2026" {
		t.Fatalf("normalized slug = %q", normalized.Slug)
	}
	derived, err := repo.UpsertPage(ctx, domain.Page{BundleID: b.ID, Title: "Derived Slug Page", Body: "# Derived Slug Page"})
	if err != nil {
		t.Fatalf("derived UpsertPage: %v", err)
	}
	if derived.Slug != "derived-slug-page" {
		t.Fatalf("derived slug = %q", derived.Slug)
	}
}

func TestBundleUpsert(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	bundle, err := repo.UpsertBundle(ctx, domain.Bundle{Slug: "Team Wiki", Title: "Team Wiki", Description: "first"})
	if err != nil {
		t.Fatalf("UpsertBundle: %v", err)
	}
	if bundle.Slug != "team-wiki" || bundle.Title != "Team Wiki" || bundle.Description != "first" {
		t.Fatalf("bundle = %+v", bundle)
	}
	updated, err := repo.UpsertBundle(ctx, domain.Bundle{Slug: "team-wiki", Title: "Team Wiki Updated", Description: "second"})
	if err != nil {
		t.Fatalf("second UpsertBundle: %v", err)
	}
	if updated.ID != bundle.ID || updated.Title != "Team Wiki Updated" || updated.Description != "second" {
		t.Fatalf("updated = %+v, original %+v", updated, bundle)
	}
	bundles, err := repo.ListBundles(ctx)
	if err != nil {
		t.Fatalf("ListBundles: %v", err)
	}
	if len(bundles) != 2 {
		t.Fatalf("bundles = %+v, want seeded plus new bundle", bundles)
	}
}

func TestStatsCountsRepositoryObjects(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	b, err := repo.GetBundle(ctx, "nanite")
	if err != nil {
		t.Fatalf("GetBundle: %v", err)
	}
	if _, err := repo.UpsertPage(ctx, domain.Page{BundleID: b.ID, Slug: "stats", Title: "Stats", Summary: "summary", Body: "# Stats\n\nSee [Runtime](runtime).", Source: "test"}); err != nil {
		t.Fatalf("UpsertPage: %v", err)
	}
	job, err := repo.CreateJob(ctx, b.ID, "wiki_page", "# Stats")
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := repo.UpdateJob(ctx, job.ID, "completed", "{}", ""); err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}
	if _, err := repo.InsertDirective(ctx, "hash", "test", "note", "Stats", 1); err != nil {
		t.Fatalf("InsertDirective: %v", err)
	}
	if _, err := repo.InsertIngest(ctx, "ingest", "text", "stats", "content"); err != nil {
		t.Fatalf("InsertIngest: %v", err)
	}
	stats, err := repo.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Bundles != 1 || stats.Pages != 1 || stats.Links != 1 || stats.Verifications != 4 {
		t.Fatalf("stats = %+v", stats)
	}
	if stats.CompileJobs != 1 || stats.CompletedJobs != 1 || stats.CompileEvents != 1 || stats.DirectiveLedger != 1 || stats.IngestLedger != 1 || stats.Templates != 1 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestUpsertPageReplacesExtractedLinks(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	b, err := repo.GetBundle(ctx, "nanite")
	if err != nil {
		t.Fatalf("GetBundle: %v", err)
	}
	page, err := repo.UpsertPage(ctx, domain.Page{
		BundleID: b.ID,
		Slug:     "links",
		Title:    "Links",
		Body:     "# Links\n\nSee [Runtime](runtime) and [[Compile Jobs]].",
	})
	if err != nil {
		t.Fatalf("UpsertPage: %v", err)
	}
	links, err := repo.ListPageLinks(ctx, page.ID)
	if err != nil {
		t.Fatalf("ListPageLinks: %v", err)
	}
	if len(links) != 2 || links[0].Target != "runtime" || links[1].Target != "compile-jobs" {
		t.Fatalf("links = %+v", links)
	}
	page, err = repo.UpsertPage(ctx, domain.Page{
		BundleID: b.ID,
		Slug:     "links",
		Title:    "Links",
		Body:     "# Links\n\nSee [Docs](https://example.com/docs).",
	})
	if err != nil {
		t.Fatalf("second UpsertPage: %v", err)
	}
	links, err = repo.ListPageLinks(ctx, page.ID)
	if err != nil {
		t.Fatalf("second ListPageLinks: %v", err)
	}
	if len(links) != 1 || links[0].Kind != "external" || links[0].Target != "https://example.com/docs" {
		t.Fatalf("replaced links = %+v", links)
	}
	bundleLinks, err := repo.ListBundleLinks(ctx, b.ID, 10)
	if err != nil {
		t.Fatalf("ListBundleLinks: %v", err)
	}
	if len(bundleLinks) != 1 || bundleLinks[0].PageID != page.ID {
		t.Fatalf("bundle links = %+v", bundleLinks)
	}
}

func TestUpsertPageReplacesVerifications(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	b, err := repo.GetBundle(ctx, "nanite")
	if err != nil {
		t.Fatalf("GetBundle: %v", err)
	}
	page, err := repo.UpsertPage(ctx, domain.Page{
		BundleID: b.ID,
		Slug:     "verified",
		Title:    "Verified",
		Summary:  "summary",
		Body:     "# Verified\n\nSee [Runtime](runtime).",
		Source:   "test",
	})
	if err != nil {
		t.Fatalf("UpsertPage: %v", err)
	}
	verifications, err := repo.ListPageVerifications(ctx, page.ID)
	if err != nil {
		t.Fatalf("ListPageVerifications: %v", err)
	}
	if len(verifications) != 4 {
		t.Fatalf("verifications = %+v, want four", verifications)
	}
	for _, verification := range verifications[:3] {
		if verification.Status != "passed" {
			t.Fatalf("verification = %+v, want passed", verification)
		}
	}
	page, err = repo.UpsertPage(ctx, domain.Page{
		BundleID: b.ID,
		Slug:     "verified",
		Title:    "Verified",
		Body:     "No matching heading.",
	})
	if err != nil {
		t.Fatalf("second UpsertPage: %v", err)
	}
	verifications, err = repo.ListPageVerifications(ctx, page.ID)
	if err != nil {
		t.Fatalf("second ListPageVerifications: %v", err)
	}
	warnings := 0
	for _, verification := range verifications {
		if verification.Status == "warning" {
			warnings++
		}
	}
	if len(verifications) != 4 || warnings != 3 {
		t.Fatalf("replaced verifications = %+v, want four with three warnings", verifications)
	}
	bundleVerifications, err := repo.ListBundleVerifications(ctx, b.ID, 10)
	if err != nil {
		t.Fatalf("ListBundleVerifications: %v", err)
	}
	if len(bundleVerifications) != 4 {
		t.Fatalf("bundle verifications = %+v", bundleVerifications)
	}
}

func TestDirectiveLedgerIdempotency(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	first, err := repo.InsertDirective(ctx, "abc", "test", "note", "hello", 1)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	second, err := repo.InsertDirective(ctx, "abc", "test", "note", "hello", 1)
	if err != nil {
		t.Fatalf("second insert: %v", err)
	}
	if !first || second {
		t.Fatalf("ledger idempotency = first %t second %t, want true false", first, second)
	}
	entry, err := repo.GetDirective(ctx, "abc")
	if err != nil {
		t.Fatalf("GetDirective: %v", err)
	}
	if entry.Hash != "abc" || entry.Source != "test" || entry.Command != "note" || entry.Line != 1 {
		t.Fatalf("entry = %+v", entry)
	}
	entries, err := repo.ListDirectives(ctx, 10)
	if err != nil {
		t.Fatalf("ListDirectives: %v", err)
	}
	if len(entries) != 1 || entries[0].Hash != "abc" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestIngestLedgerIdempotency(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	first, err := repo.InsertIngest(ctx, "hash-a", "file", "notes.md", "content-a")
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	second, err := repo.InsertIngest(ctx, "hash-a", "file", "notes.md", "content-a")
	if err != nil {
		t.Fatalf("second insert: %v", err)
	}
	if !first || second {
		t.Fatalf("ingest idempotency = first %t second %t, want true false", first, second)
	}
	b, err := repo.GetBundle(ctx, "nanite")
	if err != nil {
		t.Fatalf("GetBundle: %v", err)
	}
	job, err := repo.CreateJob(ctx, b.ID, "wiki_page", "# Notes")
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := repo.AttachIngestJob(ctx, "hash-a", job.ID); err != nil {
		t.Fatalf("AttachIngestJob: %v", err)
	}
	entry, err := repo.GetIngest(ctx, "hash-a")
	if err != nil {
		t.Fatalf("GetIngest: %v", err)
	}
	if entry.JobID != job.ID || entry.SourceKey != "notes.md" {
		t.Fatalf("entry = %+v", entry)
	}
}

func TestCompileEventsListInOrder(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	b, err := repo.GetBundle(ctx, "nanite")
	if err != nil {
		t.Fatalf("GetBundle: %v", err)
	}
	job, err := repo.CreateJob(ctx, b.ID, "wiki_page", "# Evented")
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := repo.AddEvent(ctx, job.ID, "running", "started"); err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	events, err := repo.ListEvents(ctx, job.ID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(events) != 2 || events[0].Status != "queued" || events[1].Status != "running" {
		t.Fatalf("events = %+v", events)
	}
}

func TestListJobsFiltersByBundleAndStatus(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	nanite, err := repo.GetBundle(ctx, "nanite")
	if err != nil {
		t.Fatalf("GetBundle: %v", err)
	}
	other, err := repo.UpsertBundle(ctx, domain.Bundle{Slug: "other", Title: "Other"})
	if err != nil {
		t.Fatalf("UpsertBundle: %v", err)
	}
	first, err := repo.CreateJob(ctx, nanite.ID, "wiki_page", "# First")
	if err != nil {
		t.Fatalf("CreateJob first: %v", err)
	}
	second, err := repo.CreateJob(ctx, other.ID, "wiki_page", "# Second")
	if err != nil {
		t.Fatalf("CreateJob second: %v", err)
	}
	if err := repo.UpdateJob(ctx, second.ID, "completed", "{}", ""); err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}
	jobs, err := repo.ListJobs(ctx, storage.JobFilter{BundleID: nanite.ID, Limit: 10})
	if err != nil {
		t.Fatalf("ListJobs bundle: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != first.ID {
		t.Fatalf("bundle jobs = %+v", jobs)
	}
	jobs, err = repo.ListJobs(ctx, storage.JobFilter{Status: "completed", Limit: 10})
	if err != nil {
		t.Fatalf("ListJobs status: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != second.ID {
		t.Fatalf("status jobs = %+v", jobs)
	}
}

func TestTemplateCRUD(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	seeded, err := repo.GetTemplate(ctx, "wiki_page.default")
	if err != nil {
		t.Fatalf("GetTemplate seeded: %v", err)
	}
	if seeded.Generator != "wiki_page" {
		t.Fatalf("seeded template = %+v", seeded)
	}
	tpl, err := repo.UpsertTemplate(ctx, domain.Template{Name: "wiki_page.brief", Generator: "wiki_page", Body: "# {{title}}\n\n{{summary}}"})
	if err != nil {
		t.Fatalf("UpsertTemplate: %v", err)
	}
	if tpl.Name != "wiki_page.brief" || tpl.Body == "" {
		t.Fatalf("template = %+v", tpl)
	}
	templates, err := repo.ListTemplates(ctx)
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(templates) != 2 {
		t.Fatalf("templates = %+v, want seeded plus upserted", templates)
	}
}
