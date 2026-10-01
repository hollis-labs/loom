package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hollis-labs/loom/internal/api"
	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/service"
	"github.com/hollis-labs/loom/internal/storage"
)

func TestHTTPCompileAndListPages(t *testing.T) {
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
	mux := http.NewServeMux()
	api.New(repo, service.NewCompiler(repo)).Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/compile-jobs", strings.NewReader(`{"bundle":"nanite","generator":"wiki_page","input":"# API Page\n\nBody"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("compile status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/pages?bundle=nanite&q=API", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("pages status = %d body = %s", rec.Code, rec.Body.String())
	}
	var pages []struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &pages); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(pages) != 1 || pages[0].Slug != "api-page" {
		t.Fatalf("pages = %+v", pages)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/compile-jobs", strings.NewReader(`{"bundle":"nanite","generator":"wiki_page","input":"# Zed Page\n\nBody"}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("compile second page status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/pages?bundle=nanite&limit=1", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("limited pages status = %d body = %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &pages); err != nil {
		t.Fatalf("unmarshal limited pages: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("limited pages len = %d, want 1: %+v", len(pages), pages)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/bundles/nanite/pages?limit=1", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("limited bundle pages status = %d body = %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &pages); err != nil {
		t.Fatalf("unmarshal limited bundle pages: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("limited bundle pages len = %d, want 1: %+v", len(pages), pages)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/compile-jobs", strings.NewReader(`{"bundle":"nanite","generator":"unknown","input":"Body"}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid generator status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestHTTPCompileFailureJobAndEvents(t *testing.T) {
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
	mux := http.NewServeMux()
	api.New(repo, service.NewCompiler(repo)).Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/compile-jobs", strings.NewReader(`{"bundle":"nanite","generator":"wiki_page","input":"{\"title\":\"Missing Template\",\"body\":\"Body\",\"template\":\"wiki_page.missing\"}"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("compile status = %d body = %s", rec.Code, rec.Body.String())
	}
	var job struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
		t.Fatalf("job json: %v", err)
	}
	if job.ID == 0 || job.Status != "failed" || !strings.Contains(job.Error, "load template wiki_page.missing") {
		t.Fatalf("job = %+v", job)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/compile-jobs/"+strconv.FormatInt(job.ID, 10), nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get job status = %d body = %s", rec.Code, rec.Body.String())
	}
	var fetched struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("fetched job json: %v", err)
	}
	if fetched.ID != job.ID || fetched.Status != "failed" || fetched.Error == "" {
		t.Fatalf("fetched = %+v, want failed job %d", fetched, job.ID)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/compile-jobs/"+strconv.FormatInt(job.ID, 10)+"/events", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("events status = %d body = %s", rec.Code, rec.Body.String())
	}
	var events []struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("events json: %v", err)
	}
	if len(events) != 3 || events[0].Status != "queued" || events[1].Status != "running" || events[2].Status != "failed" || !strings.Contains(events[2].Message, "wiki_page.missing") {
		t.Fatalf("events = %+v", events)
	}
}

func TestHTTPBundleCreateUpdateAndGet(t *testing.T) {
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
	mux := http.NewServeMux()
	api.New(repo, service.NewCompiler(repo)).Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/bundles", strings.NewReader(`{"slug":"team wiki","title":"Team Wiki","description":"first"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("post bundle status = %d body = %s", rec.Code, rec.Body.String())
	}
	var bundle struct {
		Slug        string `json:"slug"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &bundle); err != nil {
		t.Fatalf("post bundle json: %v", err)
	}
	if bundle.Slug != "team-wiki" || bundle.Title != "Team Wiki" {
		t.Fatalf("bundle = %+v", bundle)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/bundles/team-wiki", strings.NewReader(`{"title":"Team Wiki Updated","description":"second"}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put bundle status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/bundles/team-wiki", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get bundle status = %d body = %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &bundle); err != nil {
		t.Fatalf("get bundle json: %v", err)
	}
	if bundle.Title != "Team Wiki Updated" || bundle.Description != "second" {
		t.Fatalf("bundle = %+v", bundle)
	}
}

func TestHTTPStatusIncludesStats(t *testing.T) {
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
	mux := http.NewServeMux()
	api.New(repo, service.NewCompiler(repo)).Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/compile-jobs", strings.NewReader(`{"bundle":"nanite","generator":"wiki_page","input":"# Status Page\n\nBody"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("compile status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d body = %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Status string `json:"status"`
		Stats  struct {
			Bundles     int64 `json:"bundles"`
			Pages       int64 `json:"pages"`
			CompileJobs int64 `json:"compile_jobs"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("status json: %v", err)
	}
	if result.Status != "ok" || result.Stats.Bundles != 1 || result.Stats.Pages != 1 || result.Stats.CompileJobs != 1 {
		t.Fatalf("status = %+v", result)
	}
}

func TestHTTPPageAndBundleLinks(t *testing.T) {
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
	mux := http.NewServeMux()
	api.New(repo, service.NewCompiler(repo)).Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/compile-jobs", strings.NewReader(`{"bundle":"nanite","generator":"wiki_page","input":"{\"title\":\"Link Page\",\"body\":\"See [Runtime](runtime) and [[Compile Jobs]].\"}"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("compile status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/pages/nanite/link-page/links", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("page links status = %d body = %s", rec.Code, rec.Body.String())
	}
	var links []struct {
		Target string `json:"target"`
		Kind   string `json:"kind"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &links); err != nil {
		t.Fatalf("page links json: %v", err)
	}
	if len(links) != 2 || links[0].Target != "runtime" || links[1].Target != "compile-jobs" {
		t.Fatalf("links = %+v", links)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/bundles/nanite/links", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bundle links status = %d body = %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &links); err != nil {
		t.Fatalf("bundle links json: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("bundle links = %+v", links)
	}
}

func TestHTTPPageAndBundleVerifications(t *testing.T) {
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
	mux := http.NewServeMux()
	api.New(repo, service.NewCompiler(repo)).Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/compile-jobs", strings.NewReader(`{"bundle":"nanite","generator":"wiki_page","input":"{\"title\":\"Verify Page\",\"summary\":\"summary\",\"body\":\"Body\",\"source\":\"test\"}"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("compile status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/pages/nanite/verify-page/verifications", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("page verifications status = %d body = %s", rec.Code, rec.Body.String())
	}
	var verifications []struct {
		Kind   string `json:"kind"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &verifications); err != nil {
		t.Fatalf("page verifications json: %v", err)
	}
	if len(verifications) != 4 || verifications[0].Kind != "heading" || verifications[0].Status != "passed" {
		t.Fatalf("verifications = %+v", verifications)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/bundles/nanite/verifications", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bundle verifications status = %d body = %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &verifications); err != nil {
		t.Fatalf("bundle verifications json: %v", err)
	}
	if len(verifications) != 4 {
		t.Fatalf("bundle verifications = %+v", verifications)
	}
}

func TestHTTPPageAndBundleConformance(t *testing.T) {
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
	mux := http.NewServeMux()
	api.New(repo, service.NewCompiler(repo)).Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/compile-jobs", strings.NewReader(`{"bundle":"nanite","generator":"wiki_page","input":"{\"title\":\"Conformance Page\",\"summary\":\"summary\",\"body\":\"Body\",\"source\":\"test\"}"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("compile status = %d body = %s", rec.Code, rec.Body.String())
	}

	type report struct {
		PageID   int64 `json:"page_id"`
		Passed   bool  `json:"passed"`
		Findings []struct {
			Rule    string `json:"rule"`
			Message string `json:"message"`
		} `json:"findings"`
	}

	// Written through the normal compile+upsert path, Type is always
	// defaulted (storage.Repository.UpsertPage), so this page passes.
	req = httptest.NewRequest(http.MethodGet, "/api/pages/nanite/conformance-page/conformance", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("page conformance status = %d body = %s", rec.Code, rec.Body.String())
	}
	var got report
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("page conformance json: %v", err)
	}
	if !got.Passed || len(got.Findings) != 0 {
		t.Fatalf("page conformance = %+v, want passed with no findings", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/bundles/nanite/conformance", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bundle conformance status = %d body = %s", rec.Code, rec.Body.String())
	}
	var bundleReports []report
	if err := json.Unmarshal(rec.Body.Bytes(), &bundleReports); err != nil {
		t.Fatalf("bundle conformance json: %v", err)
	}
	if len(bundleReports) != 1 || !bundleReports[0].Passed {
		t.Fatalf("bundle conformance = %+v", bundleReports)
	}

	// Simulate a page that reached wiki_pages by some route other than
	// UpsertPage (direct SQL, a legacy row predating the OKF migration)
	// where Type's write-time default never ran, to prove the checker
	// actually catches OKF §11 violations rather than always passing now
	// that Type defaults everywhere in the normal write path.
	if _, err := repo.DB().ExecContext(ctx, `UPDATE wiki_pages SET type = '' WHERE slug = 'conformance-page'`); err != nil {
		t.Fatalf("simulate blank type: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/pages/nanite/conformance-page/conformance", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("page conformance (blank type) status = %d body = %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("page conformance (blank type) json: %v", err)
	}
	if got.Passed || len(got.Findings) == 0 {
		t.Fatalf("page conformance (blank type) = %+v, want failed with findings", got)
	}
	var sawTypeRequired bool
	for _, f := range got.Findings {
		if f.Rule == "type_required" {
			sawTypeRequired = true
		}
	}
	if !sawTypeRequired {
		t.Fatalf("findings = %+v, want a type_required finding", got.Findings)
	}
}

func TestHTTPDirectiveCompileAndJobEvents(t *testing.T) {
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
	mux := http.NewServeMux()
	api.New(repo, service.NewCompiler(repo)).Register(mux)

	body := `{"bundle":"nanite","source":"http-test","text":"::config generator=wiki_page, slug=http-directive\n::note HTTP Directive"}`
	req := httptest.NewRequest(http.MethodPost, "/api/directives/compile", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("directive compile status = %d body = %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Jobs []struct {
			ID int64 `json:"id"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("result = %+v", result)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/compile-jobs/"+strconv.FormatInt(result.Jobs[0].ID, 10)+"/events", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("events status = %d body = %s", rec.Code, rec.Body.String())
	}
	var events []struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("events json: %v", err)
	}
	if len(events) != 3 || events[0].Status != "queued" || events[2].Status != "completed" {
		t.Fatalf("events = %+v", events)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/compile-jobs/999999/events", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing job events status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/directives?limit=10", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("directives list status = %d body = %s", rec.Code, rec.Body.String())
	}
	var entries []struct {
		Hash    string `json:"hash"`
		Command string `json:"command"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("directives list json: %v", err)
	}
	if len(entries) != 1 || entries[0].Hash == "" || entries[0].Command != "note" {
		t.Fatalf("entries = %+v", entries)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/directives/"+entries[0].Hash, nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("directive get status = %d body = %s", rec.Code, rec.Body.String())
	}
	var entry struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &entry); err != nil {
		t.Fatalf("directive get json: %v", err)
	}
	if entry.Hash != entries[0].Hash {
		t.Fatalf("entry = %+v, want hash %s", entry, entries[0].Hash)
	}

	body = `{"source":"http-test","text":"::config generator=wiki_page, slug=default-bundle-directive\n::note Default Bundle Directive"}`
	req = httptest.NewRequest(http.MethodPost, "/api/directives/compile", strings.NewReader(body))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("default bundle directive status = %d body = %s", rec.Code, rec.Body.String())
	}
	page, err := repo.GetPageBySlug(ctx, "nanite", "default-bundle-directive")
	if err != nil {
		t.Fatalf("GetPageBySlug default bundle: %v", err)
	}
	if page.Title != "Default Bundle Directive" {
		t.Fatalf("page = %+v", page)
	}
}

func TestHTTPCompileJobFilters(t *testing.T) {
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
	if _, err := repo.UpsertBundle(ctx, domain.Bundle{Slug: "team", Title: "Team"}); err != nil {
		t.Fatalf("UpsertBundle: %v", err)
	}
	mux := http.NewServeMux()
	api.New(repo, service.NewCompiler(repo)).Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/compile-jobs", strings.NewReader(`{"bundle":"nanite","generator":"wiki_page","input":"# Nanite Job"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("nanite compile status = %d body = %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/compile-jobs", strings.NewReader(`{"bundle":"team","generator":"wiki_page","input":"# Team Job"}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("team compile status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/compile-jobs?bundle=team&status=completed&limit=10", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("global filtered status = %d body = %s", rec.Code, rec.Body.String())
	}
	var jobs []struct {
		BundleID int64  `json:"bundle_id"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("jobs json: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Status != "completed" {
		t.Fatalf("jobs = %+v", jobs)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/bundles/team/compile-jobs?limit=10", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bundle jobs status = %d body = %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("bundle jobs json: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("bundle jobs = %+v", jobs)
	}
}

func TestHTTPTemplates(t *testing.T) {
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
	mux := http.NewServeMux()
	api.New(repo, service.NewCompiler(repo)).Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/templates", strings.NewReader(`{"name":"wiki_page.full","generator":"wiki_page","body":"# {{title}}"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("post template status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/templates", strings.NewReader(`{"name":"bad","body":"# {{title}}"}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid template status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/templates/wiki_page.full", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get template status = %d body = %s", rec.Code, rec.Body.String())
	}
	var tpl struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tpl); err != nil {
		t.Fatalf("template json: %v", err)
	}
	if tpl.Name != "wiki_page.full" {
		t.Fatalf("template = %+v", tpl)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/templates/wiki_page.full/render", strings.NewReader(`{"title":"Rendered","body":"Body text","source":"test"}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("render template status = %d body = %s", rec.Code, rec.Body.String())
	}
	var rendered struct {
		Page struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		} `json:"page"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &rendered); err != nil {
		t.Fatalf("render json: %v", err)
	}
	if rendered.Page.Title != "Rendered" || !strings.Contains(rendered.Page.Body, "# Rendered") {
		t.Fatalf("rendered = %+v", rendered)
	}
}

func TestHTTPIngest(t *testing.T) {
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
	path := filepath.Join(dir, "api-ingest.md")
	if err := os.WriteFile(path, []byte("# API Ingest\n\nBody"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	repo := storage.NewRepository(db)
	mux := http.NewServeMux()
	api.New(repo, service.NewCompiler(repo)).Register(mux)

	body := `{"bundle":"nanite","source":"http","paths":["` + path + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/ingest", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ingest status = %d body = %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Items int `json:"items"`
		Jobs  []struct {
			Status string `json:"status"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Items != 1 || len(result.Jobs) != 1 || result.Jobs[0].Status != "completed" {
		t.Fatalf("result = %+v", result)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/ingest", strings.NewReader(`{"bundle":"nanite","source":"http","paths":[]}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid ingest status = %d body = %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/ingest?limit=10", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ingest list status = %d body = %s", rec.Code, rec.Body.String())
	}
	var entries []struct {
		Hash      string `json:"hash"`
		SourceKey string `json:"source_key"`
		JobID     int64  `json:"job_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("ledger list json: %v", err)
	}
	if len(entries) != 1 || entries[0].Hash == "" || entries[0].SourceKey != path || entries[0].JobID == 0 {
		t.Fatalf("entries = %+v", entries)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/ingest/"+entries[0].Hash, nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ingest get status = %d body = %s", rec.Code, rec.Body.String())
	}
	var entry struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &entry); err != nil {
		t.Fatalf("ledger get json: %v", err)
	}
	if entry.Hash != entries[0].Hash {
		t.Fatalf("entry = %+v, want hash %s", entry, entries[0].Hash)
	}
}

func TestHTTPIngestText(t *testing.T) {
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
	mux := http.NewServeMux()
	api.New(repo, service.NewCompiler(repo)).Register(mux)

	body := `{"bundle":"nanite","source":"chatgpt","source_key":"thread-1","title":"Thread Notes","body":"Text body"}`
	req := httptest.NewRequest(http.MethodPost, "/api/ingest/text", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ingest text status = %d body = %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Jobs []struct {
			Status string `json:"status"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("result json: %v", err)
	}
	if len(result.Jobs) != 1 || result.Jobs[0].Status != "completed" {
		t.Fatalf("result = %+v", result)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/ingest/text", strings.NewReader(`{"bundle":"nanite","body":""}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid ingest text status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestHTTPBundleExport(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dir, "loom.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := storage.NewRepository(db)
	mux := http.NewServeMux()
	api.New(repo, service.NewCompiler(repo)).WithExportRoot(filepath.Join(dir, "exports")).Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/compile-jobs", strings.NewReader(`{"bundle":"nanite","generator":"wiki_page","input":"{\"title\":\"Export Page\",\"summary\":\"summary\",\"body\":\"See [[Other Page]].\",\"source\":\"test\"}"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("compile status = %d body = %s", rec.Code, rec.Body.String())
	}

	exportDir := filepath.Join(dir, "exports", "exported")
	req = httptest.NewRequest(http.MethodPost, "/api/bundles/nanite/export", strings.NewReader(`{"dir":"`+exportDir+`"}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("export status = %d body = %s", rec.Code, rec.Body.String())
	}
	var exp struct {
		Dir   string   `json:"dir"`
		Files []string `json:"files"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &exp); err != nil {
		t.Fatalf("export json: %v", err)
	}
	if exp.Dir != exportDir {
		t.Fatalf("export dir = %q, want %q", exp.Dir, exportDir)
	}
	wantFiles := []string{"index.md", "log.md", "export-page.md"}
	if len(exp.Files) != len(wantFiles) {
		t.Fatalf("files = %+v, want %+v", exp.Files, wantFiles)
	}
	for i := range wantFiles {
		if exp.Files[i] != wantFiles[i] {
			t.Fatalf("files = %+v, want %+v", exp.Files, wantFiles)
		}
	}
	for _, name := range wantFiles {
		if _, err := os.Stat(filepath.Join(exportDir, name)); err != nil {
			t.Fatalf("exported file %s: %v", name, err)
		}
	}
	page, err := os.ReadFile(filepath.Join(exportDir, "export-page.md"))
	if err != nil {
		t.Fatalf("read page: %v", err)
	}
	if !strings.Contains(string(page), "## Links") || !strings.Contains(string(page), "## Verifications") {
		t.Fatalf("export page missing audit sections:\n%s", string(page))
	}

	req = httptest.NewRequest(http.MethodPost, "/api/bundles/nanite/export", strings.NewReader(`{`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid export json status = %d body = %s", rec.Code, rec.Body.String())
	}
}
