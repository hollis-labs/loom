// Command loom serves and operates the Loom wiki compiler.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	paths "github.com/hollis-labs/libs/core/util/apppaths"
	"github.com/hollis-labs/loom/internal/compiler"
	"github.com/hollis-labs/loom/internal/directivex"
	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/exporter"
	"github.com/hollis-labs/loom/internal/lint"
	"github.com/hollis-labs/loom/internal/storage"
)

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
		case "pages", "links", "verifications", "conformance", "compile-jobs", "export":
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
		case "links", "verifications", "conformance":
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

func (l *loom) compileCLI(ctx context.Context, args []string) error {
	args, _, err := splitAppArgs(args)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("compile", flag.ExitOnError)
	bundle := fs.String("bundle", "nanite", "bundle slug")
	generator := fs.String("generator", "wiki_page", "generator")
	input := fs.String("input", "", "input text or JSON")
	_ = fs.Parse(args)
	if *input == "" {
		data, readErr := os.ReadFile("/dev/stdin")
		if readErr != nil {
			return readErr
		}
		*input = string(data)
	}
	_, comp := l.repo, l.comp
	job, err := comp.Request(ctx, *bundle, *generator, *input)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(job)
}

func (l *loom) jobsCLI(ctx context.Context, args []string) error {
	args, _, err := splitAppArgs(args)
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
		repo := l.repo
		filter := storage.JobFilter{Status: *status, Limit: *limit}
		if strings.TrimSpace(*bundle) != "" {
			b, bundleErr := repo.GetBundle(ctx, *bundle)
			if bundleErr != nil {
				return bundleErr
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
		repo := l.repo
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

func (l *loom) bundlesCLI(ctx context.Context, args []string) error {
	args, _, err := splitAppArgs(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: loom bundles list|get|put|conformance")
	}
	switch args[0] {
	case "list":
		repo := l.repo
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
		repo := l.repo
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
		scope := fs.String("scope", "", "bundle scope (project|meta|personal)")
		_ = fs.Parse(args[1:])
		if *slug == "" || *title == "" {
			return fmt.Errorf("usage: loom bundles put -slug slug -title title [-description text] [-scope project|meta|personal]")
		}
		repo := l.repo
		bundle, err := repo.UpsertBundle(ctx, domain.Bundle{Slug: *slug, Title: *title, Description: *description, Scope: *scope})
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(bundle)
	case "conformance":
		fs := flag.NewFlagSet("bundles conformance", flag.ExitOnError)
		limit := fs.Int("limit", 100, "maximum pages to check")
		_ = fs.Parse(args[1:])
		if len(fs.Args()) != 1 {
			return fmt.Errorf("usage: loom bundles conformance [-limit 100] <slug>")
		}
		repo := l.repo
		b, err := repo.GetBundle(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		pages, err := repo.ListPages(ctx, b.ID, "", *limit)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(lint.CheckBundle(pages))
	default:
		return fmt.Errorf("usage: loom bundles list|get|put|conformance")
	}
}

func (l *loom) templatesCLI(ctx context.Context, args []string) error {
	args, _, err := splitAppArgs(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: loom templates list|get|put|render")
	}
	switch args[0] {
	case "list":
		repo := l.repo
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
		repo := l.repo
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
		repo := l.repo
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
		repo := l.repo
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

func (l *loom) pagesCLI(ctx context.Context, args []string) error {
	args, _, err := splitAppArgs(args)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("usage: loom pages list|get|links|verifications|conformance")
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("pages list", flag.ExitOnError)
		bundle := fs.String("bundle", "nanite", "bundle slug")
		query := fs.String("q", "", "search query")
		limit := fs.Int("limit", 50, "maximum pages")
		_ = fs.Parse(args[1:])
		repo := l.repo
		b, err := repo.GetBundle(ctx, *bundle)
		if err != nil {
			return err
		}
		pages, err := repo.ListPages(ctx, b.ID, *query, *limit)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(pages)
	case "get", "links", "verifications", "conformance":
		return l.pageDetailCLI(ctx, args[0], args[1:])
	default:
		return fmt.Errorf("usage: loom pages list|get|links|verifications|conformance")
	}
}

func (l *loom) pageDetailCLI(ctx context.Context, cmd string, args []string) error {
	fs := flag.NewFlagSet("pages "+cmd, flag.ExitOnError)
	bundle := fs.String("bundle", "nanite", "bundle slug")
	_ = fs.Parse(args)
	if len(fs.Args()) != 1 {
		return fmt.Errorf("usage: loom pages %s [-bundle nanite] <slug>", cmd)
	}
	repo := l.repo
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
	case "conformance":
		return json.NewEncoder(os.Stdout).Encode(lint.CheckPage(page))
	default:
		return fmt.Errorf("usage: loom pages list|get|links|verifications|conformance")
	}
}

func (l *loom) ingestCLI(ctx context.Context, args []string) error {
	args, _, err := splitAppArgs(args)
	if err != nil {
		return err
	}
	if len(args) > 0 {
		switch args[0] {
		case "list":
			return l.ingestListCLI(ctx, args[1:])
		case "get":
			return l.ingestGetCLI(ctx, args[1:])
		case "text":
			return l.ingestTextCLI(ctx, args[1:])
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
	excludePatterns, redactPatterns := l.ingestFilters(excludes, redactions)
	_, comp := l.repo, l.comp
	result, err := comp.IngestFiles(ctx, *bundle, paths, *source, excludePatterns, redactPatterns)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func (l *loom) ingestListCLI(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ingest list", flag.ExitOnError)
	limit := fs.Int("limit", 50, "maximum ledger entries")
	_ = fs.Parse(args)
	repo := l.repo
	entries, err := repo.ListIngest(ctx, *limit)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(entries)
}

func (l *loom) ingestGetCLI(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ingest get", flag.ExitOnError)
	_ = fs.Parse(args)
	if len(fs.Args()) != 1 {
		return fmt.Errorf("usage: loom ingest get <hash>")
	}
	repo := l.repo
	entry, err := repo.GetIngest(ctx, fs.Arg(0))
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(entry)
}

func (l *loom) ingestTextCLI(ctx context.Context, args []string) error {
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
	_, redactPatterns := l.ingestFilters(nil, redactions)
	_, comp := l.repo, l.comp
	result, err := comp.IngestText(ctx, *bundle, *source, *key, *title, *body, redactPatterns)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func (l *loom) ingestFilters(excludes, redactions []string) ([]string, []string) {
	outExcludes := append([]string(nil), excludes...)
	outRedactions := append([]string(nil), redactions...)
	if len(outExcludes) == 0 {
		outExcludes = append(outExcludes, l.settings.GitExclude...)
	}
	if len(outRedactions) == 0 {
		outRedactions = append(outRedactions, l.settings.RedactPatterns...)
	}
	return outExcludes, outRedactions
}

type stringListFlag []string

func (f *stringListFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *stringListFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func (l *loom) exportCLI(ctx context.Context, args []string) error {
	args, _, err := splitAppArgs(args)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	bundle := fs.String("bundle", "nanite", "bundle slug")
	dir := fs.String("dir", "", "export directory")
	_ = fs.Parse(args)
	exportDir := *dir
	if strings.TrimSpace(exportDir) == "" {
		exportDir = filepath.Join(l.settings.ExportDir, storage.Slug(*bundle))
	}
	repo := l.repo
	exp, err := exporter.ExportBundle(ctx, repo, *bundle, exportDir, l.settings.ExportDir)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(exp)
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

func (l *loom) directivesLedgerCLI(ctx context.Context, args []string) error {
	args, _, err := splitAppArgs(args)
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
		repo := l.repo
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
		repo := l.repo
		entry, err := repo.GetDirective(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(entry)
	default:
		return fmt.Errorf("usage: loom directives parse|list|get")
	}
}

// pathCLI describes the resolved layout, from the same data options the other commands got, and
// creates nothing.
func (l *loom) pathCLI([]string) error {
	entries := l.layout.Describe()
	for _, entry := range entries {
		fmt.Printf("%s\t%s\n", entry.Label, entry.Value)
	}
	return nil
}

// noDatabaseStatus is `loom status` where no database exists yet: it says so and creates nothing.
func noDatabaseStatus(path string, out io.Writer) error {
	return json.NewEncoder(out).Encode(map[string]any{
		"status":   "no_database",
		"database": path,
		"next":     "no database exists here yet; `loom migrate-data` converts an old one, and serve, mcp or a command that writes data creates a new one",
	})
}

func (l *loom) statusCLI(ctx context.Context) error {
	repo := l.repo
	stats, err := repo.Stats(ctx)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"status": "ok",
		"stats":  stats,
	})
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
			appArgs = append(appArgs, "-"+strings.TrimLeft(arg, "-"), args[i+1]) //nolint:gosec // i+1 < len(args) is checked just above; G602 false positive
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

// resolveLayout resolves the data layout and creates its directories (the commands that write
// need them). layoutFor with materialize false resolves without creating anything: help, version,
// path, status and migrate-data must leave the tree as they found it.
func resolveLayout(args []string) (paths.Layout, error) { return layoutFor(args, true) }

func layoutFor(args []string, materialize bool) (paths.Layout, error) {
	fs := flag.NewFlagSet("app", flag.ExitOnError)
	dbPath := fs.String("db", "", "sqlite database path")
	project := fs.Bool("project", true, "use project-local .loom paths")
	noProject := fs.Bool("no-project", false, "disable project-local .loom paths")
	workspace := fs.String("workspace", "", "workspace name")
	_ = fs.Parse(args)
	var opts []paths.Option
	if !materialize {
		opts = append(opts, paths.WithoutMaterialize())
	}
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
