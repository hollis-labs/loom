package mcp_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otelpropagation "go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"

	otelprop "github.com/hollis-labs/go-otel/propagation"
	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/mcp"
	"github.com/hollis-labs/loom/internal/service"
	"github.com/hollis-labs/loom/internal/storage"
)

type recordedToolCall struct {
	toolName string
	result   string
	duration time.Duration
}

type fakeToolRecorder struct {
	calls []recordedToolCall
}

func (r *fakeToolRecorder) ToolCall(_ context.Context, toolName, result string, d time.Duration) {
	r.calls = append(r.calls, recordedToolCall{toolName: toolName, result: result, duration: d})
}

func TestMCPToolsReturnDeterministicJSON(t *testing.T) {
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
	srv := mcp.NewServer(repo, service.NewCompiler(repo))

	raw, err := srv.CallTool(ctx, "loom_compile_request", map[string]any{"bundle": "nanite", "generator": "wiki_page", "input": "# MCP Page\n\nBody"})
	if err != nil {
		t.Fatalf("compile tool: %v", err)
	}
	var job struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(raw), &job); err != nil {
		t.Fatalf("compile json: %v", err)
	}
	if job.Status != "completed" {
		t.Fatalf("job = %+v", job)
	}
	if _, err := srv.CallTool(ctx, "loom_compile_request", map[string]any{"bundle": "nanite", "generator": "wiki_page", "input": "# MCP Zed\n\nBody"}); err != nil {
		t.Fatalf("second compile tool: %v", err)
	}

	raw, err = srv.CallTool(ctx, "loom_page_search", map[string]any{"bundle": "nanite", "q": "MCP", "limit": 1})
	if err != nil {
		t.Fatalf("search tool: %v", err)
	}
	var env struct {
		Count int `json:"count"`
		Items []struct {
			Slug string `json:"slug"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("search json: %v", err)
	}
	if env.Count != 1 || len(env.Items) != 1 || env.Items[0].Slug != "mcp-page" {
		t.Fatalf("env = %+v", env)
	}
}

func TestMCPBundlePutAndGet(t *testing.T) {
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
	srv := mcp.NewServer(repo, service.NewCompiler(repo))

	raw, err := srv.CallTool(ctx, "loom_bundle_put", map[string]any{"slug": "team wiki", "title": "Team Wiki", "description": "first"})
	if err != nil {
		t.Fatalf("bundle put tool: %v", err)
	}
	var bundle struct {
		Slug  string `json:"slug"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(raw), &bundle); err != nil {
		t.Fatalf("bundle put json: %v", err)
	}
	if bundle.Slug != "team-wiki" || bundle.Title != "Team Wiki" {
		t.Fatalf("bundle = %+v", bundle)
	}
	raw, err = srv.CallTool(ctx, "loom_bundle_get", map[string]any{"slug": "team-wiki"})
	if err != nil {
		t.Fatalf("bundle get tool: %v", err)
	}
	if err := json.Unmarshal([]byte(raw), &bundle); err != nil {
		t.Fatalf("bundle get json: %v", err)
	}
	if bundle.Slug != "team-wiki" {
		t.Fatalf("bundle = %+v", bundle)
	}
}

func TestMCPDefaultsDoNotLeakNilSentinel(t *testing.T) {
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
	srv := mcp.NewServer(repo, service.NewCompiler(repo))

	raw, err := srv.CallTool(ctx, "loom_compile_request", map[string]any{"input": "# Defaulted MCP\n\nBody"})
	if err != nil {
		t.Fatalf("compile tool: %v", err)
	}
	var job struct {
		BundleID int64  `json:"bundle_id"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal([]byte(raw), &job); err != nil {
		t.Fatalf("compile json: %v", err)
	}
	if job.Status != "completed" {
		t.Fatalf("job = %+v", job)
	}
	b, err := repo.GetBundle(ctx, "nanite")
	if err != nil {
		t.Fatalf("GetBundle: %v", err)
	}
	if job.BundleID != b.ID {
		t.Fatalf("job bundle = %d, want %d", job.BundleID, b.ID)
	}

	raw, err = srv.CallTool(ctx, "loom_page_search", map[string]any{"q": "Defaulted"})
	if err != nil {
		t.Fatalf("search tool: %v", err)
	}
	var env struct {
		Count int `json:"count"`
		Items []struct {
			Slug string `json:"slug"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("search json: %v", err)
	}
	if env.Count != 1 || env.Items[0].Slug != "defaulted-mcp" {
		t.Fatalf("env = %+v", env)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})
	raw, err = srv.CallTool(ctx, "loom_export_bundle", map[string]any{})
	if err != nil {
		t.Fatalf("export tool: %v", err)
	}
	var exp struct {
		Dir string `json:"dir"`
	}
	if err := json.Unmarshal([]byte(raw), &exp); err != nil {
		t.Fatalf("export json: %v", err)
	}
	if exp.Dir != filepath.Join(".", "exports", "nanite") {
		t.Fatalf("export dir = %q", exp.Dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "<nil>")); !os.IsNotExist(err) {
		t.Fatalf("unexpected <nil> export dir err = %v", err)
	}
}

func TestMCPDefaultedArgumentsAreOptionalInSchemas(t *testing.T) {
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
	srv := mcp.NewServer(repo, service.NewCompiler(repo))

	want := map[string][]string{
		"loom_page_search":             nil,
		"loom_page_get":                {"slug"},
		"loom_bundle_links":            nil,
		"loom_compile_request":         {"input"},
		"loom_compile_from_directives": {"text"},
		"loom_ingest_files":            {"paths"},
		"loom_ingest_text":             {"body"},
		"loom_export_bundle":           nil,
	}
	for _, def := range srv.ToolDefinitions() {
		required, ok := want[def.Name]
		if !ok {
			continue
		}
		got := requiredFields(t, def.InputSchema)
		if !sameStrings(got, required) {
			t.Fatalf("%s required = %+v, want %+v", def.Name, got, required)
		}
		delete(want, def.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing schema checks for %+v", want)
	}
}

func TestMCPStatus(t *testing.T) {
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
	srv := mcp.NewServer(repo, service.NewCompiler(repo))
	if _, err := srv.CallTool(ctx, "loom_compile_request", map[string]any{"bundle": "nanite", "generator": "wiki_page", "input": "# Status Page\n\nBody"}); err != nil {
		t.Fatalf("compile: %v", err)
	}
	raw, err := srv.CallTool(ctx, "loom_status", map[string]any{})
	if err != nil {
		t.Fatalf("status tool: %v", err)
	}
	var result struct {
		Status string `json:"status"`
		Stats  struct {
			Bundles     int64 `json:"bundles"`
			Pages       int64 `json:"pages"`
			CompileJobs int64 `json:"compile_jobs"`
		} `json:"stats"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("status json: %v", err)
	}
	if result.Status != "ok" || result.Stats.Bundles != 1 || result.Stats.Pages != 1 || result.Stats.CompileJobs != 1 {
		t.Fatalf("status = %+v", result)
	}
}

func TestMCPToolCallsEmitOTelSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(otelpropagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(noop.NewTracerProvider())
		otel.SetTextMapPropagator(otelpropagation.NewCompositeTextMapPropagator())
		_ = tp.Shutdown(context.Background())
	})

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
	srv := mcp.NewServer(repo, service.NewCompiler(repo))

	if _, err := srv.CallTool(ctx, "loom_status", map[string]any{}); err != nil {
		t.Fatalf("status tool: %v", err)
	}
	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if spans[0].Name() != "hollis.tool.call" {
		t.Fatalf("span name = %q", spans[0].Name())
	}
	if got := spanAttr(spans[0].Attributes(), "hollis.tool.name"); got != "loom_status" {
		t.Fatalf("tool attr = %q", got)
	}
}

func TestMCPToolTraceparentBecomesParentSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(otelpropagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(noop.NewTracerProvider())
		otel.SetTextMapPropagator(otelpropagation.NewCompositeTextMapPropagator())
		_ = tp.Shutdown(context.Background())
	})

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
	srv := mcp.NewServer(repo, service.NewCompiler(repo))

	parentCtx, parent := otel.Tracer("loom-test").Start(ctx, "parent")
	args := otelprop.InjectMCP(parentCtx, map[string]any{})
	parent.End()

	if _, err := srv.CallTool(ctx, "loom_status", args); err != nil {
		t.Fatalf("status tool: %v", err)
	}
	spans := recorder.Ended()
	if len(spans) != 2 {
		t.Fatalf("spans = %d, want parent plus tool", len(spans))
	}
	toolSpan := spanByName(t, spans, "hollis.tool.call")
	parentSpan := spanByName(t, spans, "parent")
	if toolSpan.SpanContext().TraceID() != parentSpan.SpanContext().TraceID() {
		t.Fatalf("tool trace id = %s, parent trace id = %s", toolSpan.SpanContext().TraceID(), parentSpan.SpanContext().TraceID())
	}
	if toolSpan.Parent().SpanID() != parentSpan.SpanContext().SpanID() {
		t.Fatalf("tool parent = %s, want %s", toolSpan.Parent().SpanID(), parentSpan.SpanContext().SpanID())
	}
}

func TestMCPToolErrorsMarkOTelSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(noop.NewTracerProvider())
		_ = tp.Shutdown(context.Background())
	})

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
	srv := mcp.NewServer(repo, service.NewCompiler(repo))

	if _, err := srv.CallTool(ctx, "loom_bundle_get", map[string]any{"slug": "missing"}); err == nil {
		t.Fatalf("bundle get missing returned nil error")
	}
	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if spans[0].Status().Code != codes.Error {
		t.Fatalf("span status = %+v, want error", spans[0].Status())
	}
	if len(spans[0].Events()) == 0 {
		t.Fatalf("span events empty, want recorded error")
	}
}

func TestMCPToolCallsRecordMetrics(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	recorder := &fakeToolRecorder{}
	repo := storage.NewRepository(db)
	srv := mcp.NewServerWithOptions(repo, service.NewCompiler(repo), mcp.Options{Recorder: recorder})

	if _, err := srv.CallTool(ctx, "loom_status", map[string]any{}); err != nil {
		t.Fatalf("status tool: %v", err)
	}
	if _, err := srv.CallTool(ctx, "loom_bundle_get", map[string]any{"slug": "missing"}); err == nil {
		t.Fatalf("bundle get missing returned nil error")
	}
	if len(recorder.calls) != 2 {
		t.Fatalf("calls = %+v, want 2", recorder.calls)
	}
	if recorder.calls[0].toolName != "loom_status" || recorder.calls[0].result != "ok" || recorder.calls[0].duration <= 0 {
		t.Fatalf("first call = %+v", recorder.calls[0])
	}
	if recorder.calls[1].toolName != "loom_bundle_get" || recorder.calls[1].result != "error" || recorder.calls[1].duration <= 0 {
		t.Fatalf("second call = %+v", recorder.calls[1])
	}
}

func TestMCPPageLinks(t *testing.T) {
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
	srv := mcp.NewServer(repo, service.NewCompiler(repo))

	_, err = srv.CallTool(ctx, "loom_compile_request", map[string]any{"bundle": "nanite", "generator": "wiki_page", "input": `{"title":"MCP Links","body":"See [Runtime](runtime) and [[Compile Jobs]]."}`})
	if err != nil {
		t.Fatalf("compile tool: %v", err)
	}
	raw, err := srv.CallTool(ctx, "loom_page_links", map[string]any{"bundle": "nanite", "slug": "mcp-links"})
	if err != nil {
		t.Fatalf("page links tool: %v", err)
	}
	var env struct {
		Count int `json:"count"`
		Items []struct {
			Target string `json:"target"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("links json: %v", err)
	}
	if env.Count != 2 || env.Items[0].Target != "runtime" || env.Items[1].Target != "compile-jobs" {
		t.Fatalf("env = %+v", env)
	}
	raw, err = srv.CallTool(ctx, "loom_bundle_links", map[string]any{"bundle": "nanite", "limit": float64(10)})
	if err != nil {
		t.Fatalf("bundle links tool: %v", err)
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("bundle links json: %v", err)
	}
	if env.Count != 2 {
		t.Fatalf("bundle env = %+v", env)
	}
}

func TestMCPPageVerifications(t *testing.T) {
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
	srv := mcp.NewServer(repo, service.NewCompiler(repo))

	_, err = srv.CallTool(ctx, "loom_compile_request", map[string]any{"bundle": "nanite", "generator": "wiki_page", "input": `{"title":"MCP Verify","summary":"summary","body":"Body","source":"test"}`})
	if err != nil {
		t.Fatalf("compile tool: %v", err)
	}
	raw, err := srv.CallTool(ctx, "loom_page_verifications", map[string]any{"bundle": "nanite", "slug": "mcp-verify"})
	if err != nil {
		t.Fatalf("page verifications tool: %v", err)
	}
	var env struct {
		Count int `json:"count"`
		Items []struct {
			Kind   string `json:"kind"`
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("verifications json: %v", err)
	}
	if env.Count != 4 || env.Items[0].Kind != "heading" || env.Items[0].Status != "passed" {
		t.Fatalf("env = %+v", env)
	}
	raw, err = srv.CallTool(ctx, "loom_bundle_verifications", map[string]any{"bundle": "nanite", "limit": float64(10)})
	if err != nil {
		t.Fatalf("bundle verifications tool: %v", err)
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("bundle verifications json: %v", err)
	}
	if env.Count != 4 {
		t.Fatalf("bundle env = %+v", env)
	}
}

func TestMCPCompileFromDirectivesAndJobGet(t *testing.T) {
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
	srv := mcp.NewServer(repo, service.NewCompiler(repo))

	raw, err := srv.CallTool(ctx, "loom_compile_from_directives", map[string]any{
		"bundle": "nanite",
		"source": "mcp-test",
		"text":   "::config generator=wiki_page, slug=mcp-directive\n::note MCP Directive",
	})
	if err != nil {
		t.Fatalf("compile directives tool: %v", err)
	}
	var result struct {
		Jobs []struct {
			ID int64 `json:"id"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("compile directives json: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("result = %+v", result)
	}
	raw, err = srv.CallTool(ctx, "loom_compile_job_get", map[string]any{"id": float64(result.Jobs[0].ID)})
	if err != nil {
		t.Fatalf("job get tool: %v", err)
	}
	var detail struct {
		Events []struct {
			Status string `json:"status"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(raw), &detail); err != nil {
		t.Fatalf("job get json: %v", err)
	}
	if len(detail.Events) != 3 || detail.Events[2].Status != "completed" {
		t.Fatalf("detail = %+v", detail)
	}

	raw, err = srv.CallTool(ctx, "loom_directive_list", map[string]any{"limit": float64(10)})
	if err != nil {
		t.Fatalf("directive list tool: %v", err)
	}
	var entries struct {
		Count int `json:"count"`
		Items []struct {
			Hash    string `json:"hash"`
			Command string `json:"command"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		t.Fatalf("directive list json: %v", err)
	}
	if entries.Count != 1 || entries.Items[0].Hash == "" || entries.Items[0].Command != "note" {
		t.Fatalf("entries = %+v", entries)
	}
	raw, err = srv.CallTool(ctx, "loom_directive_get", map[string]any{"hash": entries.Items[0].Hash})
	if err != nil {
		t.Fatalf("directive get tool: %v", err)
	}
	var entry struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		t.Fatalf("directive get json: %v", err)
	}
	if entry.Hash != entries.Items[0].Hash {
		t.Fatalf("entry = %+v, want hash %s", entry, entries.Items[0].Hash)
	}
}

func TestMCPCompileFailureJobGet(t *testing.T) {
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
	srv := mcp.NewServer(repo, service.NewCompiler(repo))

	raw, err := srv.CallTool(ctx, "loom_compile_request", map[string]any{
		"bundle":    "nanite",
		"generator": "wiki_page",
		"input":     `{"title":"Missing Template","body":"Body","template":"wiki_page.missing"}`,
	})
	if err != nil {
		t.Fatalf("compile tool: %v", err)
	}
	var job struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &job); err != nil {
		t.Fatalf("compile json: %v", err)
	}
	if job.ID == 0 || job.Status != "failed" || job.Error == "" {
		t.Fatalf("job = %+v, want failed job", job)
	}

	raw, err = srv.CallTool(ctx, "loom_compile_job_get", map[string]any{"id": float64(job.ID)})
	if err != nil {
		t.Fatalf("job get tool: %v", err)
	}
	var detail struct {
		Job struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
			Error  string `json:"error"`
		} `json:"job"`
		Events []struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"events"`
	}
	if err := json.Unmarshal([]byte(raw), &detail); err != nil {
		t.Fatalf("job get json: %v", err)
	}
	if detail.Job.ID != job.ID || detail.Job.Status != "failed" || detail.Job.Error == "" {
		t.Fatalf("detail job = %+v, want failed job %d", detail.Job, job.ID)
	}
	if len(detail.Events) != 3 || detail.Events[0].Status != "queued" || detail.Events[1].Status != "running" || detail.Events[2].Status != "failed" || detail.Events[2].Message == "" {
		t.Fatalf("detail events = %+v", detail.Events)
	}
}

func TestMCPCompileJobListFilters(t *testing.T) {
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
	srv := mcp.NewServer(repo, service.NewCompiler(repo))
	if _, err := srv.CallTool(ctx, "loom_compile_request", map[string]any{"bundle": "nanite", "generator": "wiki_page", "input": "# Nanite Job"}); err != nil {
		t.Fatalf("nanite compile: %v", err)
	}
	if _, err := srv.CallTool(ctx, "loom_compile_request", map[string]any{"bundle": "team", "generator": "wiki_page", "input": "# Team Job"}); err != nil {
		t.Fatalf("team compile: %v", err)
	}
	raw, err := srv.CallTool(ctx, "loom_compile_job_list", map[string]any{"bundle": "team", "status": "completed", "limit": float64(10)})
	if err != nil {
		t.Fatalf("job list tool: %v", err)
	}
	var env struct {
		Count int `json:"count"`
		Items []struct {
			Status string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("job list json: %v", err)
	}
	if env.Count != 1 || env.Items[0].Status != "completed" {
		t.Fatalf("env = %+v", env)
	}
}

func TestMCPTemplateTools(t *testing.T) {
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
	srv := mcp.NewServer(repo, service.NewCompiler(repo))

	raw, err := srv.CallTool(ctx, "loom_template_put", map[string]any{"name": "wiki_page.compact", "generator": "wiki_page", "body": "# {{title}}"})
	if err != nil {
		t.Fatalf("template put: %v", err)
	}
	var tpl struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(raw), &tpl); err != nil {
		t.Fatalf("put json: %v", err)
	}
	if tpl.Name != "wiki_page.compact" {
		t.Fatalf("tpl = %+v", tpl)
	}

	raw, err = srv.CallTool(ctx, "loom_template_list", map[string]any{})
	if err != nil {
		t.Fatalf("template list: %v", err)
	}
	var env struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("list json: %v", err)
	}
	if env.Count != 2 {
		t.Fatalf("env = %+v, want seeded plus new template", env)
	}

	raw, err = srv.CallTool(ctx, "loom_template_render", map[string]any{"name": "wiki_page.compact", "title": "Rendered", "body": "Body text"})
	if err != nil {
		t.Fatalf("template render: %v", err)
	}
	var rendered struct {
		Page struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		} `json:"page"`
	}
	if err := json.Unmarshal([]byte(raw), &rendered); err != nil {
		t.Fatalf("render json: %v", err)
	}
	if rendered.Page.Title != "Rendered" || rendered.Page.Body != "# Rendered" {
		t.Fatalf("rendered = %+v", rendered)
	}
}

func TestMCPIngestFiles(t *testing.T) {
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
	path := filepath.Join(dir, "mcp-ingest.md")
	if err := os.WriteFile(path, []byte("# MCP Ingest\n\nBody"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	repo := storage.NewRepository(db)
	srv := mcp.NewServer(repo, service.NewCompiler(repo))

	raw, err := srv.CallTool(ctx, "loom_ingest_files", map[string]any{"bundle": "nanite", "source": "mcp", "paths": []any{path}})
	if err != nil {
		t.Fatalf("ingest tool: %v", err)
	}
	var result struct {
		Items int `json:"items"`
		Jobs  []struct {
			Status string `json:"status"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("ingest json: %v", err)
	}
	if result.Items != 1 || len(result.Jobs) != 1 || result.Jobs[0].Status != "completed" {
		t.Fatalf("result = %+v", result)
	}

	raw, err = srv.CallTool(ctx, "loom_ingest_list", map[string]any{"limit": float64(5)})
	if err != nil {
		t.Fatalf("ingest list tool: %v", err)
	}
	var env struct {
		Count int `json:"count"`
		Items []struct {
			Hash      string `json:"hash"`
			SourceKey string `json:"source_key"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("ingest list json: %v", err)
	}
	if env.Count != 1 || env.Items[0].Hash == "" || env.Items[0].SourceKey != path {
		t.Fatalf("env = %+v", env)
	}

	raw, err = srv.CallTool(ctx, "loom_ingest_get", map[string]any{"hash": env.Items[0].Hash})
	if err != nil {
		t.Fatalf("ingest get tool: %v", err)
	}
	var entry struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		t.Fatalf("ingest get json: %v", err)
	}
	if entry.Hash != env.Items[0].Hash {
		t.Fatalf("entry = %+v, want hash %s", entry, env.Items[0].Hash)
	}
}

func TestMCPIngestText(t *testing.T) {
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
	srv := mcp.NewServer(repo, service.NewCompiler(repo))

	raw, err := srv.CallTool(ctx, "loom_ingest_text", map[string]any{"bundle": "nanite", "source": "chatgpt", "source_key": "thread-1", "title": "Thread Notes", "body": "Text body"})
	if err != nil {
		t.Fatalf("ingest text tool: %v", err)
	}
	var result struct {
		Jobs []struct {
			Status string `json:"status"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatalf("ingest text json: %v", err)
	}
	if len(result.Jobs) != 1 || result.Jobs[0].Status != "completed" {
		t.Fatalf("result = %+v", result)
	}
}

func TestMCPExportBundle(t *testing.T) {
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
	srv := mcp.NewServer(repo, service.NewCompiler(repo))

	if _, err := srv.CallTool(ctx, "loom_compile_request", map[string]any{"bundle": "nanite", "generator": "wiki_page", "input": `{"title":"MCP Export","summary":"summary","body":"See [[Other Page]].","source":"test"}`}); err != nil {
		t.Fatalf("compile tool: %v", err)
	}

	exportDir := filepath.Join(dir, "exported")
	raw, err := srv.CallTool(ctx, "loom_export_bundle", map[string]any{"bundle": "nanite", "dir": exportDir})
	if err != nil {
		t.Fatalf("export tool: %v", err)
	}
	var exp struct {
		Dir   string   `json:"dir"`
		Files []string `json:"files"`
	}
	if err := json.Unmarshal([]byte(raw), &exp); err != nil {
		t.Fatalf("export json: %v", err)
	}
	if exp.Dir != exportDir {
		t.Fatalf("export dir = %q, want %q", exp.Dir, exportDir)
	}
	wantFiles := []string{"index.md", "log.md", "mcp-export.md"}
	if len(exp.Files) != len(wantFiles) {
		t.Fatalf("files = %+v, want %+v", exp.Files, wantFiles)
	}
	for i := range wantFiles {
		if exp.Files[i] != wantFiles[i] {
			t.Fatalf("files = %+v, want %+v", exp.Files, wantFiles)
		}
		if _, err := os.Stat(filepath.Join(exportDir, wantFiles[i])); err != nil {
			t.Fatalf("exported file %s: %v", wantFiles[i], err)
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})
	raw, err = srv.CallTool(ctx, "loom_export_bundle", map[string]any{"bundle": "nanite"})
	if err != nil {
		t.Fatalf("default export tool: %v", err)
	}
	if err := json.Unmarshal([]byte(raw), &exp); err != nil {
		t.Fatalf("default export json: %v", err)
	}
	if exp.Dir != filepath.Join(".", "exports", "nanite") {
		t.Fatalf("default export dir = %q", exp.Dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "exports", "nanite", "mcp-export.md")); err != nil {
		t.Fatalf("default exported page: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "<nil>")); !os.IsNotExist(err) {
		t.Fatalf("unexpected <nil> export dir err = %v", err)
	}
}

func spanAttr(attrs []attribute.KeyValue, key string) string {
	for _, attr := range attrs {
		if string(attr.Key) == key {
			return attr.Value.AsString()
		}
	}
	return ""
}

func spanByName(t *testing.T, spans []trace.ReadOnlySpan, name string) trace.ReadOnlySpan {
	t.Helper()
	for _, span := range spans {
		if span.Name() == name {
			return span
		}
	}
	t.Fatalf("span %q not found in %+v", name, spans)
	return nil
}

func requiredFields(t *testing.T, schema any) []string {
	t.Helper()
	obj, ok := schema.(map[string]interface{})
	if !ok {
		t.Fatalf("schema = %#v, want object schema", schema)
	}
	raw, ok := obj["required"]
	if !ok {
		return nil
	}
	items, ok := raw.([]string)
	if !ok {
		t.Fatalf("required = %#v, want []string", raw)
	}
	return items
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
