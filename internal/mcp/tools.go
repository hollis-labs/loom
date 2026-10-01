package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hollis-labs/go-mcp/budget"
	gmcp "github.com/hollis-labs/go-mcp/server"
	hotel "github.com/hollis-labs/go-otel"
	otelprop "github.com/hollis-labs/go-otel/propagation"
	loomcompiler "github.com/hollis-labs/loom/internal/compiler"
	"github.com/hollis-labs/loom/internal/directivex"
	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/exporter"
	"github.com/hollis-labs/loom/internal/lint"
	"github.com/hollis-labs/loom/internal/service"
	"github.com/hollis-labs/loom/internal/storage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type ToolRecorder interface {
	ToolCall(ctx context.Context, toolName, result string, d time.Duration)
}

type Options struct {
	ExportRoot string
	Recorder   ToolRecorder
}

func NewServer(repo *storage.Repository, compiler *service.Compiler) *gmcp.Server {
	return NewServerWithOptions(repo, compiler, Options{})
}

func NewServerWithOptions(repo *storage.Repository, compiler *service.Compiler, opts Options) *gmcp.Server {
	srv := gmcp.NewServer("loom", "0.1.0")
	obj := gmcp.ObjectSchema
	str := func() map[string]any { return map[string]any{"type": "string"} }
	// cache is the per-caller wiki_* result cache (CW-20260816-0014): search/
	// list tools store their full, pre-budget-limit result here when it's
	// byte-large, and loom_fetch_result / loom_search_result let a caller
	// deep-dive into that full result by id afterward. repo.DB() is the same
	// connection every other repository method already uses.
	cache := NewResultCache(repo.DB(), ResultCacheConfig{})

	registerTool(srv, opts, gmcp.Tool{Name: "loom_status", Description: "Get Loom repository status and object counts.", InputSchema: gmcp.EmptyObjectSchema(), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		stats, err := repo.Stats(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{"status": "ok", "stats": stats}, nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_bundle_list", Description: "List Loom wiki bundles.", InputSchema: obj(map[string]any{"caller_id": str()}), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		items, err := repo.ListBundles(ctx)
		if err != nil {
			return nil, err
		}
		return cacheListResult(cache, stringArg(args, "caller_id", "default"), "loom_bundle_list", items, budget.Config{}, "%d bundles found."), nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_bundle_get", Description: "Get one Loom wiki bundle.", InputSchema: obj(map[string]any{"slug": str()}, "slug"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		item, err := repo.GetBundle(ctx, stringArg(args, "slug", ""))
		if err != nil {
			return nil, err
		}
		return item, nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_bundle_put", Description: "Create or update a Loom wiki bundle.", InputSchema: obj(map[string]any{"slug": str(), "title": str(), "description": str(), "scope": str()}, "slug", "title"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		item, err := repo.UpsertBundle(ctx, domain.Bundle{
			Slug:        stringArg(args, "slug", ""),
			Title:       stringArg(args, "title", ""),
			Description: stringArg(args, "description", ""),
			Scope:       stringArg(args, "scope", ""),
		})
		if err != nil {
			return nil, err
		}
		return item, nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_page_search", Description: "Search pages in a bundle.", InputSchema: obj(map[string]any{"bundle": str(), "q": str(), "limit": map[string]any{"type": "number"}, "caller_id": str()}), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		bundle := stringArg(args, "bundle", "nanite")
		q := stringArg(args, "q", "")
		limit := numericArg(args["limit"], 25)
		b, err := repo.GetBundle(ctx, bundle)
		if err != nil {
			return nil, err
		}
		items, err := repo.ListPages(ctx, b.ID, q, limit)
		if err != nil {
			return nil, err
		}
		return cacheListResult(cache, stringArg(args, "caller_id", "default"), "loom_page_search", items, budget.Config{}, "%d pages found. Use loom_page_get for full content."), nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_page_get", Description: "Get one page by bundle and slug.", InputSchema: obj(map[string]any{"bundle": str(), "slug": str()}, "slug"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		p, err := repo.GetPageBySlug(ctx, stringArg(args, "bundle", "nanite"), stringArg(args, "slug", ""))
		if err != nil {
			return nil, err
		}
		return p, nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_page_links", Description: "List links extracted from one page.", InputSchema: obj(map[string]any{"bundle": str(), "slug": str(), "caller_id": str()}, "slug"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		p, err := repo.GetPageBySlug(ctx, stringArg(args, "bundle", "nanite"), stringArg(args, "slug", ""))
		if err != nil {
			return nil, err
		}
		links, err := repo.ListPageLinks(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		return cacheListResult(cache, stringArg(args, "caller_id", "default"), "loom_page_links", links, budget.Config{}, "%d page links found."), nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_bundle_links", Description: "List links extracted across a bundle.", InputSchema: obj(map[string]any{"bundle": str(), "limit": map[string]any{"type": "number"}, "caller_id": str()}), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		b, err := repo.GetBundle(ctx, stringArg(args, "bundle", "nanite"))
		if err != nil {
			return nil, err
		}
		limit := 100
		if v, ok := args["limit"]; ok {
			id, err := numericID(v)
			if err != nil {
				return nil, err
			}
			limit = int(id)
		}
		links, err := repo.ListBundleLinks(ctx, b.ID, limit)
		if err != nil {
			return nil, err
		}
		return cacheListResult(cache, stringArg(args, "caller_id", "default"), "loom_bundle_links", links, budget.Config{}, "%d bundle links found."), nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_page_verifications", Description: "List deterministic verification records for one page.", InputSchema: obj(map[string]any{"bundle": str(), "slug": str(), "caller_id": str()}, "slug"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		p, err := repo.GetPageBySlug(ctx, stringArg(args, "bundle", "nanite"), stringArg(args, "slug", ""))
		if err != nil {
			return nil, err
		}
		verifications, err := repo.ListPageVerifications(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		return cacheListResult(cache, stringArg(args, "caller_id", "default"), "loom_page_verifications", verifications, budget.Config{}, "%d page verifications found."), nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_bundle_verifications", Description: "List deterministic verification records across a bundle.", InputSchema: obj(map[string]any{"bundle": str(), "limit": map[string]any{"type": "number"}, "caller_id": str()}), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		b, err := repo.GetBundle(ctx, stringArg(args, "bundle", "nanite"))
		if err != nil {
			return nil, err
		}
		limit := 100
		if v, ok := args["limit"]; ok {
			id, err := numericID(v)
			if err != nil {
				return nil, err
			}
			limit = int(id)
		}
		verifications, err := repo.ListBundleVerifications(ctx, b.ID, limit)
		if err != nil {
			return nil, err
		}
		return cacheListResult(cache, stringArg(args, "caller_id", "default"), "loom_bundle_verifications", verifications, budget.Config{}, "%d bundle verifications found."), nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_page_conformance", Description: "Check one page against OKF §11 structural conformance (parseable frontmatter + a type field) - distinct from loom_page_verifications' content checks.", InputSchema: obj(map[string]any{"bundle": str(), "slug": str()}, "slug"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		p, err := repo.GetPageBySlug(ctx, stringArg(args, "bundle", "nanite"), stringArg(args, "slug", ""))
		if err != nil {
			return nil, err
		}
		return lint.CheckPage(p), nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_bundle_conformance", Description: "Check every page in a bundle against OKF §11 structural conformance (parseable frontmatter + a type field) - distinct from loom_bundle_verifications' content checks.", InputSchema: obj(map[string]any{"bundle": str(), "limit": map[string]any{"type": "number"}}), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		b, err := repo.GetBundle(ctx, stringArg(args, "bundle", "nanite"))
		if err != nil {
			return nil, err
		}
		limit := numericArg(args["limit"], 100)
		pages, err := repo.ListPages(ctx, b.ID, "", limit)
		if err != nil {
			return nil, err
		}
		reports := lint.CheckBundle(pages)
		return budget.Apply(reports, budget.Config{}, "%d bundle conformance reports found."), nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_compile_request", Description: "Compile and store a wiki page.", InputSchema: obj(map[string]any{"bundle": str(), "generator": str(), "input": str()}, "input"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		job, err := compiler.Request(ctx, stringArg(args, "bundle", "nanite"), stringArg(args, "generator", "wiki_page"), stringArg(args, "input", ""))
		if err != nil {
			return nil, err
		}
		return job, nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_compile_from_directives", Description: "Parse directives, ledger hashes, and compile new wiki-page directives.", InputSchema: obj(map[string]any{"bundle": str(), "text": str(), "source": str()}, "text"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		resp, err := compiler.RequestFromDirectives(ctx, stringArg(args, "bundle", "nanite"), stringArg(args, "text", ""), stringArg(args, "source", ""))
		if err != nil {
			return nil, err
		}
		return resp, nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_compile_job_get", Description: "Get one compile job and its event log.", InputSchema: obj(map[string]any{"id": map[string]any{"type": "number"}}, "id"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		id, err := numericID(args["id"])
		if err != nil {
			return nil, err
		}
		job, err := repo.GetJob(ctx, id)
		if err != nil {
			return nil, err
		}
		events, err := repo.ListEvents(ctx, id)
		if err != nil {
			return nil, err
		}
		return map[string]any{"job": job, "events": events}, nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_compile_job_list", Description: "List compile jobs, optionally filtered by bundle and status.", InputSchema: obj(map[string]any{"bundle": str(), "status": str(), "limit": map[string]any{"type": "number"}, "caller_id": str()}), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		filter := storage.JobFilter{Status: stringArg(args, "status", ""), Limit: numericArg(args["limit"], 50)}
		if bundle := stringArg(args, "bundle", ""); bundle != "" {
			b, err := repo.GetBundle(ctx, bundle)
			if err != nil {
				return nil, err
			}
			filter.BundleID = b.ID
		}
		jobs, err := repo.ListJobs(ctx, filter)
		if err != nil {
			return nil, err
		}
		return cacheListResult(cache, stringArg(args, "caller_id", "default"), "loom_compile_job_list", jobs, budget.Config{}, "%d compile jobs found."), nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_ingest_files", Description: "Ingest markdown/text files into a bundle through deterministic wiki-page compile jobs.", InputSchema: obj(map[string]any{
		"bundle":          str(),
		"paths":           map[string]any{"type": "array", "items": str()},
		"source":          str(),
		"exclude_paths":   map[string]any{"type": "array", "items": str()},
		"redact_patterns": map[string]any{"type": "array", "items": str()},
	}, "paths"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		result, err := compiler.IngestFiles(ctx, stringArg(args, "bundle", "nanite"), stringSliceArg(args["paths"]), stringArg(args, "source", "file"), stringSliceArg(args["exclude_paths"]), stringSliceArg(args["redact_patterns"]))
		if err != nil {
			return nil, err
		}
		return result, nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_ingest_text", Description: "Ingest direct text into a bundle through a deterministic wiki-page compile job.", InputSchema: obj(map[string]any{
		"bundle":          str(),
		"source":          str(),
		"source_key":      str(),
		"title":           str(),
		"body":            str(),
		"redact_patterns": map[string]any{"type": "array", "items": str()},
	}, "body"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		result, err := compiler.IngestText(ctx, stringArg(args, "bundle", "nanite"), stringArg(args, "source", "text"), stringArg(args, "source_key", ""), stringArg(args, "title", ""), stringArg(args, "body", ""), stringSliceArg(args["redact_patterns"]))
		if err != nil {
			return nil, err
		}
		return result, nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_ingest_list", Description: "List recent ingest ledger entries.", InputSchema: obj(map[string]any{"limit": map[string]any{"type": "number"}, "caller_id": str()}), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		limit := 50
		if v, ok := args["limit"]; ok {
			id, err := numericID(v)
			if err != nil {
				return nil, err
			}
			limit = int(id)
		}
		entries, err := repo.ListIngest(ctx, limit)
		if err != nil {
			return nil, err
		}
		return cacheListResult(cache, stringArg(args, "caller_id", "default"), "loom_ingest_list", entries, budget.Config{}, "%d ingest entries found."), nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_ingest_get", Description: "Get one ingest ledger entry by hash.", InputSchema: obj(map[string]any{"hash": str()}, "hash"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		entry, err := repo.GetIngest(ctx, stringArg(args, "hash", ""))
		if err != nil {
			return nil, err
		}
		return entry, nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_export_bundle", Description: "Export a bundle as OKF-style markdown files.", InputSchema: obj(map[string]any{"bundle": str(), "dir": str()}), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		exp, err := exporter.ExportBundle(ctx, repo, stringArg(args, "bundle", "nanite"), stringArg(args, "dir", ""), opts.ExportRoot)
		if err != nil {
			return nil, err
		}
		return exp, nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_directives_parse", Description: "Parse Loom directives from text.", InputSchema: obj(map[string]any{"text": str(), "source": str(), "save": map[string]any{"type": "boolean"}}, "text"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		text, _ := args["text"].(string)
		source, _ := args["source"].(string)
		save, _ := args["save"].(bool)
		var (
			resp directivex.ParseResponse
			err  error
		)
		if save {
			resp, err = directivex.ParseAndLedger(ctx, repo, text, source)
		} else {
			resp = directivex.Parse(text, source)
		}
		if err != nil {
			return nil, err
		}
		return resp, nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_directive_list", Description: "List directive ledger entries.", InputSchema: obj(map[string]any{"limit": map[string]any{"type": "number"}, "caller_id": str()}), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		limit := 50
		if v, ok := args["limit"]; ok {
			id, err := numericID(v)
			if err != nil {
				return nil, err
			}
			limit = int(id)
		}
		entries, err := repo.ListDirectives(ctx, limit)
		if err != nil {
			return nil, err
		}
		return cacheListResult(cache, stringArg(args, "caller_id", "default"), "loom_directive_list", entries, budget.Config{}, "%d directive ledger entries found."), nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_directive_get", Description: "Get one directive ledger entry by hash.", InputSchema: obj(map[string]any{"hash": str()}, "hash"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		entry, err := repo.GetDirective(ctx, stringArg(args, "hash", ""))
		if err != nil {
			return nil, err
		}
		return entry, nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_template_list", Description: "List generator templates.", InputSchema: obj(map[string]any{"caller_id": str()}), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		items, err := repo.ListTemplates(ctx)
		if err != nil {
			return nil, err
		}
		return cacheListResult(cache, stringArg(args, "caller_id", "default"), "loom_template_list", items, budget.Config{}, "%d templates found."), nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_template_get", Description: "Get one generator template by name.", InputSchema: obj(map[string]any{"name": str()}, "name"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		tpl, err := repo.GetTemplate(ctx, stringArg(args, "name", ""))
		if err != nil {
			return nil, err
		}
		return tpl, nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_template_put", Description: "Create or update a generator template.", InputSchema: obj(map[string]any{"name": str(), "generator": str(), "body": str()}, "name", "generator"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		tpl, err := repo.UpsertTemplate(ctx, domain.Template{
			Name:      stringArg(args, "name", ""),
			Generator: stringArg(args, "generator", "wiki_page"),
			Body:      stringArg(args, "body", ""),
		})
		if err != nil {
			return nil, err
		}
		return tpl, nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_template_render", Description: "Preview a stored template render without storing a page.", InputSchema: obj(map[string]any{"name": str(), "title": str(), "summary": str(), "body": str(), "source": str()}, "name", "title"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		tpl, err := repo.GetTemplate(ctx, stringArg(args, "name", ""))
		if err != nil {
			return nil, err
		}
		input, err := json.Marshal(loomcompiler.Request{
			Title:   stringArg(args, "title", "Untitled Page"),
			Summary: stringArg(args, "summary", ""),
			Body:    stringArg(args, "body", ""),
			Source:  stringArg(args, "source", ""),
		})
		if err != nil {
			return nil, err
		}
		result, err := loomcompiler.CompileWikiPageWithTemplate(0, string(input), tpl.Body)
		if err != nil {
			return nil, err
		}
		return result, nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_fetch_result", Description: "Deep-dive into a wiki_result://<id> pointer left in a truncated search/list tool's hint - retrieves a byte slice of the full, untruncated cached result.", InputSchema: obj(map[string]any{
		"id":        str(),
		"caller_id": str(),
		"offset":    map[string]any{"type": "number"},
		"length":    map[string]any{"type": "number"},
	}, "id"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		id := stringArg(args, "id", "")
		if id == "" {
			return nil, fmt.Errorf("id is required")
		}
		callerID := stringArg(args, "caller_id", "default")
		offset := numericArg(args["offset"], 0)
		length := numericArg(args["length"], 0)
		slice, totalSize, err := cache.Fetch(callerID, id, offset, length)
		if err != nil {
			return nil, err
		}
		return map[string]any{"id": id, "offset": offset, "total_size": totalSize, "data": slice}, nil
	}})
	registerTool(srv, opts, gmcp.Tool{Name: "loom_search_result", Description: "Regex-search a wiki_result://<id> pointer left in a truncated search/list tool's hint, returning matching lines with context - use to locate one specific item (e.g. by slug) inside a large cached result without paging through it by offset.", InputSchema: obj(map[string]any{
		"id":          str(),
		"caller_id":   str(),
		"pattern":     str(),
		"max_matches": map[string]any{"type": "number"},
	}, "id", "pattern"), Handler: func(ctx context.Context, args map[string]any) (any, error) {
		id := stringArg(args, "id", "")
		pattern := stringArg(args, "pattern", "")
		if id == "" || pattern == "" {
			return nil, fmt.Errorf("id and pattern are required")
		}
		callerID := stringArg(args, "caller_id", "default")
		maxMatches := numericArg(args["max_matches"], 20)
		matches, err := cache.Search(callerID, id, pattern, maxMatches)
		if err != nil {
			return nil, err
		}
		return map[string]any{"id": id, "pattern": pattern, "matches": matches}, nil
	}})
	return srv
}

func registerTool(srv *gmcp.Server, opts Options, tool gmcp.Tool) {
	hints, ok := toolAnnotations[tool.Name]
	if !ok {
		panic(fmt.Sprintf("mcp: tool %q has no entry in toolAnnotations (tool_annotations.go) - add one deliberately rather than shipping silently-defaulted hints", tool.Name))
	}
	tool.ReadOnlyHint = hints.ReadOnly
	tool.DestructiveHint = hints.Destructive
	tool.IdempotentHint = hints.Idempotent
	tool.OpenWorldHint = false
	tool.Handler = traceTool(tool.Name, tool.Handler, opts.Recorder)
	srv.RegisterTool(tool)
}

func traceTool(name string, handler gmcp.ToolHandler, recorder ToolRecorder) gmcp.ToolHandler {
	return func(ctx context.Context, args map[string]any) (any, error) {
		if args != nil {
			extracted := otelprop.ExtractMCP(args)
			ctx = contextWithTrace(ctx, extracted)
		}
		ctx, span := hotel.ToolCallSpan(ctx, name)
		defer span.End()
		start := time.Now()
		out, err := handler(ctx, args)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			if recorder != nil {
				recorder.ToolCall(ctx, name, "error", time.Since(start))
			}
			return nil, err
		}
		if recorder != nil {
			recorder.ToolCall(ctx, name, "ok", time.Since(start))
		}
		return out, nil
	}
}

func contextWithTrace(parent, extracted context.Context) context.Context {
	if parent == nil {
		return extracted
	}
	if extracted == nil {
		return parent
	}
	sc := trace.SpanContextFromContext(extracted)
	if !sc.IsValid() {
		return parent
	}
	return trace.ContextWithRemoteSpanContext(parent, sc)
}

func numericID(v any) (int64, error) {
	switch n := v.(type) {
	case int:
		return int64(n), nil
	case int64:
		return n, nil
	case float64:
		return int64(n), nil
	default:
		return 0, fmt.Errorf("invalid numeric id %v", v)
	}
}

func stringArg(args map[string]any, key, fallback string) string {
	value, _ := args[key].(string)
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func stringSliceArg(v any) []string {
	switch items := v.(type) {
	case []string:
		return items
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if strings.TrimSpace(items) == "" {
			return nil
		}
		return []string{items}
	default:
		return nil
	}
}

func numericArg(v any, fallback int) int {
	if v == nil {
		return fallback
	}
	id, err := numericID(v)
	if err != nil || id <= 0 {
		return fallback
	}
	return int(id)
}
