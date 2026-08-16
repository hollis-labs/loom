// Command loom serves and operates the Loom wiki compiler.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	paths "github.com/hollis-labs/go-apppaths/paths"
	httptransport "github.com/hollis-labs/go-mcp/transport/http"
	"github.com/hollis-labs/loom/internal/api"
	"github.com/hollis-labs/loom/internal/compiler"
	loomconfig "github.com/hollis-labs/loom/internal/config"
	"github.com/hollis-labs/loom/internal/directivex"
	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/exporter"
	loommcp "github.com/hollis-labs/loom/internal/mcp"
	loomotel "github.com/hollis-labs/loom/internal/otel"
	loomserver "github.com/hollis-labs/loom/internal/server"
	"github.com/hollis-labs/loom/internal/service"
	"github.com/hollis-labs/loom/internal/storage"
	"github.com/hollis-labs/loom/internal/webui"
)

const version = "0.1.0"

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
		args = args[1:]
	}
	switch cmd {
	case "serve":
		return serve(ctx, args)
	case "migrate":
		_, _, cleanup, err := openApp(ctx, args)
		if cleanup != nil {
			defer cleanup()
		}
		return err
	case "compile":
		return compileCLI(ctx, args)
	case "jobs":
		return jobsCLI(ctx, args)
	case "bundles":
		return bundlesCLI(ctx, args)
	case "templates":
		return templatesCLI(ctx, args)
	case "pages":
		return pagesCLI(ctx, args)
	case "ingest":
		return ingestCLI(ctx, args)
	case "export":
		return exportCLI(ctx, args)
	case "directives":
		if len(args) > 0 && args[0] == "parse" {
			return directivesCLI(args[1:])
		}
		return directivesLedgerCLI(ctx, args)
	case "mcp":
		return mcpCLI(ctx, args)
	case "path":
		return pathCLI(args)
	case "version":
		fmt.Println(version)
		return nil
	case "config":
		return configCLI(args)
	case "status":
		return statusCLI(ctx, args)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func mcpCLI(ctx context.Context, args []string) error {
	args, appArgs, err := splitAppArgs(args)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	otelEnabled := fs.Bool("otel", loomotel.EnabledFromEnv(), "enable OpenTelemetry export")
	otelMetrics := fs.Bool("otel-metrics", loomotel.MetricsEnabledFromEnv(), "enable OpenTelemetry MCP tool metrics")
	otelEndpoint := fs.String("otel-endpoint", "", "OTLP HTTP endpoint host:port")
	_ = fs.Parse(args)
	repo, comp, cleanup, err := openApp(ctx, appArgs)
	if err != nil {
		return err
	}
	defer cleanup()
	otelRuntime, err := loomotel.InitRuntime(ctx, loomotel.Config{
		Enabled:        *otelEnabled,
		MetricsEnabled: *otelMetrics,
		ServiceName:    "loom",
		ServiceVersion: version,
		Environment:    "development",
		Endpoint:       *otelEndpoint,
	})
	if err != nil {
		return err
	}
	defer otelRuntime.Shutdown()
	return loommcp.NewServerWithOptions(repo, comp, loommcp.Options{Recorder: otelRuntime.Recorder}).Run()
}

func serve(ctx context.Context, args []string) error {
	args, appArgs, err := splitAppArgs(args)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "HTTP listen address")
	otelEnabled := fs.Bool("otel", loomotel.EnabledFromEnv(), "enable OpenTelemetry export")
	otelMetrics := fs.Bool("otel-metrics", loomotel.MetricsEnabledFromEnv(), "enable OpenTelemetry HTTP metrics")
	otelEndpoint := fs.String("otel-endpoint", "", "OTLP HTTP endpoint host:port")
	_ = fs.Parse(args)
	repo, comp, cleanup, err := openApp(ctx, appArgs)
	if err != nil {
		return err
	}
	defer cleanup()
	otelRuntime, err := loomotel.InitRuntime(ctx, loomotel.Config{
		Enabled:        *otelEnabled,
		MetricsEnabled: *otelMetrics,
		ServiceName:    "loom",
		ServiceVersion: version,
		Environment:    "development",
		Endpoint:       *otelEndpoint,
	})
	if err != nil {
		return err
	}
	defer otelRuntime.Shutdown()

	mux := http.NewServeMux()
	api.New(repo, comp).Register(mux)
	mux.Handle("/mcp", httptransport.NewHandler(loommcp.NewServerWithOptions(repo, comp, loommcp.Options{Recorder: otelRuntime.Recorder}), httptransport.HandlerOptions{}))
	webui.Mount(mux)

	log.Printf("Loom listening on http://localhost%s/", *addr)
	return loomserver.Serve(ctx, loomserver.New(loomserver.Config{Addr: *addr, Handler: mux, Recorder: otelRuntime.Recorder, RouteResolver: loomRoute}))
}

func loomRoute(r *http.Request) string {
	path := strings.Trim(r.URL.Path, "/")
	if path == "" {
		return "/"
	}
	parts := strings.Split(path, "/")
	switch {
	case parts[0] == "mcp":
		return "/mcp"
	case parts[0] != "api":
		return "/webui"
	case len(parts) == 2:
		return "/api/" + parts[1]
	case len(parts) >= 3 && parts[1] == "bundles":
		if len(parts) == 3 {
			return "/api/bundles/{bundle}"
		}
		switch parts[3] {
		case "pages", "links", "verifications", "compile-jobs", "export":
			return "/api/bundles/{bundle}/" + parts[3]
		default:
			return "/api/bundles/{bundle}/*"
		}
	case len(parts) >= 3 && parts[1] == "pages":
		if len(parts) < 4 {
			return "/api/pages/*"
		}
		if len(parts) == 4 {
			return "/api/pages/{bundle}/{slug}"
		}
		switch parts[4] {
		case "links", "verifications":
			return "/api/pages/{bundle}/{slug}/" + parts[4]
		default:
			return "/api/pages/{bundle}/{slug}/*"
		}
	case len(parts) >= 3 && parts[1] == "compile-jobs":
		if len(parts) == 3 {
			return "/api/compile-jobs/{id}"
		}
		if len(parts) == 4 && parts[3] == "events" {
			return "/api/compile-jobs/{id}/events"
		}
		return "/api/compile-jobs/{id}/*"
	case len(parts) >= 3 && parts[1] == "ingest":
		if parts[2] == "text" {
			return "/api/ingest/text"
		}
		return "/api/ingest/{hash}"
	case len(parts) >= 3 && parts[1] == "directives":
		switch parts[2] {
		case "parse", "compile":
			return "/api/directives/" + parts[2]
		default:
			return "/api/directives/{hash}"
		}
	case len(parts) >= 3 && parts[1] == "templates":
		if len(parts) == 3 {
			return "/api/templates/{name}"
		}
		if len(parts) == 4 && parts[3] == "render" {
			return "/api/templates/{name}/render"
		}
		return "/api/templates/{name}/*"
	default:
		return "/api/*"
	}
}

func compileCLI(ctx context.Context, args []string) error {
	args, appArgs, err := splitAppArgs(args)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("compile", flag.ExitOnError)
	bundle := fs.String("bundle", "nanite", "bundle slug")
	generator := fs.String("generator", "wiki_page", "generator")
	input := fs.String("input", "", "input text or JSON")
	_ = fs.Parse(args)
	if *input == "" {
		data, err := os.ReadFile("/dev/stdin")
		if err != nil {
			return err
		}
		*input = string(data)
	}
	_, comp, cleanup, err := openApp(ctx, appArgs)
	if err != nil {
		return err
	}
	defer cleanup()
	job, err := comp.Request(ctx, *bundle, *generator, *input)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(job)
}

func jobsCLI(ctx context.Context, args []string) error {
	args, appArgs, err := splitAppArgs(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: loom jobs list|get")
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("jobs list", flag.ExitOnError)
		bundle := fs.String("bundle", "", "bundle slug")
		status := fs.String("status", "", "job status")
		limit := fs.Int("limit", 50, "maximum jobs")
		_ = fs.Parse(args[1:])
		repo, _, cleanup, err := openApp(ctx, appArgs)
		if err != nil {
			return err
		}
		defer cleanup()
		filter := storage.JobFilter{Status: *status, Limit: *limit}
		if strings.TrimSpace(*bundle) != "" {
			b, err := repo.GetBundle(ctx, *bundle)
			if err != nil {
				return err
			}
			filter.BundleID = b.ID
		}
		jobs, err := repo.ListJobs(ctx, filter)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(jobs)
	case "get":
		fs := flag.NewFlagSet("jobs get", flag.ExitOnError)
		_ = fs.Parse(args[1:])
		if len(fs.Args()) != 1 {
			return fmt.Errorf("usage: loom jobs get <id>")
		}
		id, err := strconv.ParseInt(fs.Arg(0), 10, 64)
		if err != nil {
			return err
		}
		repo, _, cleanup, err := openApp(ctx, appArgs)
		if err != nil {
			return err
		}
		defer cleanup()
		job, err := repo.GetJob(ctx, id)
		if err != nil {
			return err
		}
		events, err := repo.ListEvents(ctx, id)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"job": job, "events": events})
	default:
		return fmt.Errorf("usage: loom jobs list|get")
	}
}

func bundlesCLI(ctx context.Context, args []string) error {
	args, appArgs, err := splitAppArgs(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: loom bundles list|get|put")
	}
	switch args[0] {
	case "list":
		repo, _, cleanup, err := openApp(ctx, appArgs)
		if err != nil {
			return err
		}
		defer cleanup()
		bundles, err := repo.ListBundles(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(bundles)
	case "get":
		fs := flag.NewFlagSet("bundles get", flag.ExitOnError)
		_ = fs.Parse(args[1:])
		if len(fs.Args()) != 1 {
			return fmt.Errorf("usage: loom bundles get <slug>")
		}
		repo, _, cleanup, err := openApp(ctx, appArgs)
		if err != nil {
			return err
		}
		defer cleanup()
		bundle, err := repo.GetBundle(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(bundle)
	case "put":
		fs := flag.NewFlagSet("bundles put", flag.ExitOnError)
		slug := fs.String("slug", "", "bundle slug")
		title := fs.String("title", "", "bundle title")
		description := fs.String("description", "", "bundle description")
		_ = fs.Parse(args[1:])
		if *slug == "" || *title == "" {
			return fmt.Errorf("usage: loom bundles put -slug slug -title title [-description text]")
		}
		repo, _, cleanup, err := openApp(ctx, appArgs)
		if err != nil {
			return err
		}
		defer cleanup()
		bundle, err := repo.UpsertBundle(ctx, domain.Bundle{Slug: *slug, Title: *title, Description: *description})
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(bundle)
	default:
		return fmt.Errorf("usage: loom bundles list|get|put")
	}
}

func templatesCLI(ctx context.Context, args []string) error {
	args, appArgs, err := splitAppArgs(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: loom templates list|get|put|render")
	}
	switch args[0] {
	case "list":
		repo, _, cleanup, err := openApp(ctx, appArgs)
		if err != nil {
			return err
		}
		defer cleanup()
		templates, err := repo.ListTemplates(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(templates)
	case "get":
		fs := flag.NewFlagSet("templates get", flag.ExitOnError)
		_ = fs.Parse(args[1:])
		if len(fs.Args()) != 1 {
			return fmt.Errorf("usage: loom templates get <name>")
		}
		repo, _, cleanup, err := openApp(ctx, appArgs)
		if err != nil {
			return err
		}
		defer cleanup()
		tpl, err := repo.GetTemplate(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(tpl)
	case "put":
		fs := flag.NewFlagSet("templates put", flag.ExitOnError)
		name := fs.String("name", "", "template name")
		generator := fs.String("generator", "wiki_page", "generator name")
		body := fs.String("body", "", "template body")
		_ = fs.Parse(args[1:])
		if *name == "" || *body == "" {
			return fmt.Errorf("usage: loom templates put -name name [-generator wiki_page] -body body")
		}
		repo, _, cleanup, err := openApp(ctx, appArgs)
		if err != nil {
			return err
		}
		defer cleanup()
		tpl, err := repo.UpsertTemplate(ctx, domain.Template{Name: *name, Generator: *generator, Body: *body})
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(tpl)
	case "render":
		fs := flag.NewFlagSet("templates render", flag.ExitOnError)
		name := fs.String("name", "wiki_page.default", "template name")
		title := fs.String("title", "Untitled Page", "page title")
		summary := fs.String("summary", "", "page summary")
		body := fs.String("body", "", "page body")
		source := fs.String("source", "", "page source")
		_ = fs.Parse(args[1:])
		repo, _, cleanup, err := openApp(ctx, appArgs)
		if err != nil {
			return err
		}
		defer cleanup()
		tpl, err := repo.GetTemplate(ctx, *name)
		if err != nil {
			return err
		}
		input, err := json.Marshal(compiler.Request{Title: *title, Summary: *summary, Body: *body, Source: *source})
		if err != nil {
			return err
		}
		result, err := compiler.CompileWikiPageWithTemplate(0, string(input), tpl.Body)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	default:
		return fmt.Errorf("usage: loom templates list|get|put|render")
	}
}

func pagesCLI(ctx context.Context, args []string) error {
	args, appArgs, err := splitAppArgs(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: loom pages list|get|links|verifications")
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("pages list", flag.ExitOnError)
		bundle := fs.String("bundle", "nanite", "bundle slug")
		query := fs.String("q", "", "search query")
		limit := fs.Int("limit", 50, "maximum pages")
		_ = fs.Parse(args[1:])
		repo, _, cleanup, err := openApp(ctx, appArgs)
		if err != nil {
			return err
		}
		defer cleanup()
		b, err := repo.GetBundle(ctx, *bundle)
		if err != nil {
			return err
		}
		pages, err := repo.ListPages(ctx, b.ID, *query, *limit)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(pages)
	case "get", "links", "verifications":
		return pageDetailCLI(ctx, appArgs, args[0], args[1:])
	default:
		return fmt.Errorf("usage: loom pages list|get|links|verifications")
	}
}

func pageDetailCLI(ctx context.Context, appArgs []string, cmd string, args []string) error {
	fs := flag.NewFlagSet("pages "+cmd, flag.ExitOnError)
	bundle := fs.String("bundle", "nanite", "bundle slug")
	_ = fs.Parse(args)
	if len(fs.Args()) != 1 {
		return fmt.Errorf("usage: loom pages %s [-bundle nanite] <slug>", cmd)
	}
	repo, _, cleanup, err := openApp(ctx, appArgs)
	if err != nil {
		return err
	}
	defer cleanup()
	page, err := repo.GetPageBySlug(ctx, *bundle, fs.Arg(0))
	if err != nil {
		return err
	}
	switch cmd {
	case "get":
		return json.NewEncoder(os.Stdout).Encode(page)
	case "links":
		links, err := repo.ListPageLinks(ctx, page.ID)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(links)
	case "verifications":
		verifications, err := repo.ListPageVerifications(ctx, page.ID)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(verifications)
	default:
		return fmt.Errorf("usage: loom pages list|get|links|verifications")
	}
}

func ingestCLI(ctx context.Context, args []string) error {
	args, appArgs, err := splitAppArgs(args)
	if err != nil {
		return err
	}
	if len(args) > 0 {
		switch args[0] {
		case "list":
			return ingestListCLI(ctx, appArgs, args[1:])
		case "get":
			return ingestGetCLI(ctx, appArgs, args[1:])
		case "text":
			return ingestTextCLI(ctx, appArgs, args[1:])
		}
	}
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	bundle := fs.String("bundle", "nanite", "bundle slug")
	source := fs.String("source", "file", "source name")
	var paths, excludes, redactions stringListFlag
	fs.Var(&paths, "path", "file or directory path to ingest; repeatable")
	fs.Var(&excludes, "exclude", "path substring to skip; repeatable")
	fs.Var(&redactions, "redact", "regexp pattern to replace with [REDACTED]; repeatable")
	_ = fs.Parse(args)
	paths = append(paths, fs.Args()...)
	if len(paths) == 0 {
		return fmt.Errorf("usage: loom ingest [-bundle nanite] [-source file] -path file-or-dir")
	}
	excludePatterns, redactPatterns, err := configuredIngestFilters(appArgs, excludes, redactions)
	if err != nil {
		return err
	}
	_, comp, cleanup, err := openApp(ctx, appArgs)
	if err != nil {
		return err
	}
	defer cleanup()
	result, err := comp.IngestFiles(ctx, *bundle, paths, *source, excludePatterns, redactPatterns)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func ingestListCLI(ctx context.Context, appArgs []string, args []string) error {
	fs := flag.NewFlagSet("ingest list", flag.ExitOnError)
	limit := fs.Int("limit", 50, "maximum ledger entries")
	_ = fs.Parse(args)
	repo, _, cleanup, err := openApp(ctx, appArgs)
	if err != nil {
		return err
	}
	defer cleanup()
	entries, err := repo.ListIngest(ctx, *limit)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(entries)
}

func ingestGetCLI(ctx context.Context, appArgs []string, args []string) error {
	fs := flag.NewFlagSet("ingest get", flag.ExitOnError)
	_ = fs.Parse(args)
	if len(fs.Args()) != 1 {
		return fmt.Errorf("usage: loom ingest get <hash>")
	}
	repo, _, cleanup, err := openApp(ctx, appArgs)
	if err != nil {
		return err
	}
	defer cleanup()
	entry, err := repo.GetIngest(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(entry)
}

func ingestTextCLI(ctx context.Context, appArgs []string, args []string) error {
	fs := flag.NewFlagSet("ingest text", flag.ExitOnError)
	bundle := fs.String("bundle", "nanite", "bundle slug")
	source := fs.String("source", "text", "source name")
	key := fs.String("key", "", "source key")
	title := fs.String("title", "", "page title")
	body := fs.String("body", "", "text body")
	var redactions stringListFlag
	fs.Var(&redactions, "redact", "regexp pattern to replace with [REDACTED]; repeatable")
	_ = fs.Parse(args)
	if strings.TrimSpace(*body) == "" {
		data, err := os.ReadFile("/dev/stdin")
		if err != nil {
			return err
		}
		*body = string(data)
	}
	_, redactPatterns, err := configuredIngestFilters(appArgs, nil, redactions)
	if err != nil {
		return err
	}
	_, comp, cleanup, err := openApp(ctx, appArgs)
	if err != nil {
		return err
	}
	defer cleanup()
	result, err := comp.IngestText(ctx, *bundle, *source, *key, *title, *body, redactPatterns)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func configuredIngestFilters(appArgs []string, excludes, redactions []string) ([]string, []string, error) {
	layout, err := resolveLayout(appArgs)
	if err != nil {
		return nil, nil, err
	}
	cfg, _, err := loomconfig.Load(layout, "config.yaml")
	if err != nil {
		return nil, nil, err
	}
	outExcludes := append([]string(nil), excludes...)
	outRedactions := append([]string(nil), redactions...)
	if len(outExcludes) == 0 {
		outExcludes = append(outExcludes, cfg.Filters.GitExcludePath...)
	}
	if len(outRedactions) == 0 {
		outRedactions = append(outRedactions, cfg.Filters.RedactPatterns...)
	}
	return outExcludes, outRedactions, nil
}

type stringListFlag []string

func (f *stringListFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *stringListFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func exportCLI(ctx context.Context, args []string) error {
	args, appArgs, err := splitAppArgs(args)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	bundle := fs.String("bundle", "nanite", "bundle slug")
	dir := fs.String("dir", "", "export directory")
	_ = fs.Parse(args)
	exportDir := *dir
	if strings.TrimSpace(exportDir) == "" {
		var err error
		exportDir, err = defaultExportDir(appArgs, *bundle)
		if err != nil {
			return err
		}
	}
	repo, _, cleanup, err := openApp(ctx, appArgs)
	if err != nil {
		return err
	}
	defer cleanup()
	exp, err := exporter.ExportBundle(ctx, repo, *bundle, exportDir)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(exp)
}

func defaultExportDir(args []string, bundle string) (string, error) {
	layout, err := resolveLayout(args)
	if err != nil {
		return "", err
	}
	cfg, _, err := loomconfig.Load(layout, "config.yaml")
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg.Paths.ExportDir, storage.Slug(bundle)), nil
}

func directivesCLI(args []string) error {
	return directivesCLIWithIO(args, os.Stdin, os.Stdout)
}

func directivesCLIWithIO(args []string, in io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet("directives parse", flag.ExitOnError)
	source := fs.String("source", "stdin", "source identifier")
	_ = fs.Parse(args)
	data, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(directivex.Parse(string(data), *source))
}

func directivesLedgerCLI(ctx context.Context, args []string) error {
	args, appArgs, err := splitAppArgs(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: loom directives parse|list|get")
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("directives list", flag.ExitOnError)
		limit := fs.Int("limit", 50, "maximum ledger entries")
		_ = fs.Parse(args[1:])
		repo, _, cleanup, err := openApp(ctx, appArgs)
		if err != nil {
			return err
		}
		defer cleanup()
		entries, err := repo.ListDirectives(ctx, *limit)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(entries)
	case "get":
		fs := flag.NewFlagSet("directives get", flag.ExitOnError)
		_ = fs.Parse(args[1:])
		if len(fs.Args()) != 1 {
			return fmt.Errorf("usage: loom directives get <hash>")
		}
		repo, _, cleanup, err := openApp(ctx, appArgs)
		if err != nil {
			return err
		}
		defer cleanup()
		entry, err := repo.GetDirective(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(entry)
	default:
		return fmt.Errorf("usage: loom directives parse|list|get")
	}
}

func pathCLI(args []string) error {
	layout, err := resolveLayout(args)
	if err != nil {
		return err
	}
	entries := layout.Describe()
	for _, entry := range entries {
		fmt.Printf("%s\t%s\n", entry.Label, entry.Value)
	}
	return nil
}

func configCLI(args []string) error {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	projectFile := fs.String("config", "config.yaml", "project config file")
	_ = fs.Parse(args)
	layout, err := resolveLayout(fs.Args())
	if err != nil {
		return err
	}
	cfg, loaded, err := loomconfig.Load(layout, *projectFile)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"config": cfg,
		"loaded": loaded,
	})
}

func statusCLI(ctx context.Context, args []string) error {
	repo, _, cleanup, err := openApp(ctx, args)
	if err != nil {
		return err
	}
	defer cleanup()
	stats, err := repo.Stats(ctx)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"status": "ok",
		"stats":  stats,
	})
}

func openApp(ctx context.Context, args []string) (*storage.Repository, *service.Compiler, func(), error) {
	layout, err := resolveLayout(args)
	if err != nil {
		return nil, nil, nil, err
	}
	db, err := storage.Open(ctx, layout.MainDB())
	if err != nil {
		return nil, nil, nil, err
	}
	if err := storage.Migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, nil, nil, err
	}
	repo := storage.NewRepository(db)
	return repo, service.NewCompiler(repo), func() { _ = db.Close() }, nil
}

func splitAppArgs(args []string) ([]string, []string, error) {
	cmdArgs := make([]string, 0, len(args))
	appArgs := make([]string, 0)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--db" || arg == "-db" || arg == "--workspace" || arg == "-workspace":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("%s requires a value", arg)
			}
			appArgs = append(appArgs, "-"+strings.TrimLeft(arg, "-"), args[i+1])
			i++
		case strings.HasPrefix(arg, "--db=") || strings.HasPrefix(arg, "-db=") || strings.HasPrefix(arg, "--workspace=") || strings.HasPrefix(arg, "-workspace="):
			appArgs = append(appArgs, "-"+strings.TrimLeft(arg, "-"))
		case arg == "--project" || arg == "-project":
			appArgs = append(appArgs, "-project=true")
		case arg == "--no-project":
			appArgs = append(appArgs, "-project=false")
		case strings.HasPrefix(arg, "--project=") || strings.HasPrefix(arg, "-project="):
			appArgs = append(appArgs, "-"+strings.TrimLeft(arg, "-"))
		default:
			cmdArgs = append(cmdArgs, arg)
		}
	}
	return cmdArgs, appArgs, nil
}

func resolveLayout(args []string) (paths.Layout, error) {
	fs := flag.NewFlagSet("app", flag.ExitOnError)
	dbPath := fs.String("db", "", "sqlite database path")
	project := fs.Bool("project", true, "use project-local .loom paths")
	noProject := fs.Bool("no-project", false, "disable project-local .loom paths")
	workspace := fs.String("workspace", "", "workspace name")
	_ = fs.Parse(args)
	var opts []paths.Option
	if *noProject {
		*project = false
	}
	if *project {
		opts = append(opts, paths.WithProjectMode())
	}
	if strings.TrimSpace(*dbPath) != "" {
		opts = append(opts, paths.WithDBOverride(*dbPath))
	}
	if strings.TrimSpace(*workspace) != "" {
		opts = append(opts, paths.WithWorkspace(*workspace))
	}
	return paths.Resolve("loom", opts...)
}
