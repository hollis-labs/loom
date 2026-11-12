package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"path/filepath"

	"github.com/spf13/cobra"

	paths "github.com/hollis-labs/libs/core/util/apppaths"
	"github.com/hollis-labs/substrate/module"
	"github.com/hollis-labs/substrate/modules/auth"
	"github.com/hollis-labs/substrate/modules/database"
	"github.com/hollis-labs/substrate/modules/httphost"
	"github.com/hollis-labs/substrate/modules/identity"
	"github.com/hollis-labs/substrate/modules/mcp"
	"github.com/hollis-labs/substrate/modules/secrets"

	"github.com/hollis-labs/loom/internal/api"
	"github.com/hollis-labs/loom/internal/hostguard"
	loomanthropic "github.com/hollis-labs/loom/internal/llm/anthropic"
	loommcp "github.com/hollis-labs/loom/internal/mcp"
	loomotel "github.com/hollis-labs/loom/internal/otel"
	loomserver "github.com/hollis-labs/loom/internal/server"
	"github.com/hollis-labs/loom/internal/service"
	"github.com/hollis-labs/loom/internal/settings"
	"github.com/hollis-labs/loom/internal/storage"
	"github.com/hollis-labs/loom/internal/webui"
)

// loom is what the commands share: the started framework app, the repository and the compiler
// over its database, and Loom's own settings. ready, when set, learns the bound address (tests
// listen on port 0).
type loom struct {
	app      *module.App
	repo     *storage.Repository
	comp     *service.Compiler
	settings settings.Settings
	otel     loomotel.Runtime
	ready    func(net.Addr)
}

// newLoom builds the repository and compiler over the database module's store, reads the
// "loom" settings block, wires the LLM provider when a key reference is configured and
// registers the MCP tools on the mcp module's server.
func newLoom(ctx context.Context, app *module.App, layout paths.Layout) (*loom, error) {
	st := database.From(app)
	if st == nil {
		return nil, errors.New("the database module must be started")
	}
	cfg, err := settings.Parse(app.Config[settings.Key], filepath.Join(layout.DataDir(), "exports"))
	if err != nil {
		return nil, err
	}
	repo := storage.NewRepositoryRW(st.DB(), st.Reader())
	comp := service.NewCompiler(repo)
	if err = configureLLMProvider(ctx, app, comp, cfg); err != nil {
		return nil, err
	}
	otelRuntime, err := loomotel.InitRuntime(ctx, loomotel.Config{
		Enabled:        cfg.OTelEnabled,
		MetricsEnabled: cfg.OTelMetrics,
		ServiceName:    "loom",
		ServiceVersion: version,
		Environment:    "development",
		Endpoint:       cfg.OTelEndpoint,
	})
	if err != nil {
		return nil, err
	}
	srv := mcp.Server(app)
	if srv == nil {
		return nil, errors.New("the mcp module must be started")
	}
	scopes := auth.ScopeTable(app)
	for _, tool := range loommcp.ToolNames() {
		scopes.Tool(tool, loommcp.ToolScope(tool))
	}
	opts := mcpOptions(otelRuntime, cfg.ExportDir)
	opts.Authorize = func(ctx context.Context, tool string) error { return mcp.Authorize(ctx, app, tool) }
	loommcp.RegisterTools(srv, repo, comp, opts)
	return &loom{app: app, repo: repo, comp: comp, settings: cfg, otel: otelRuntime}, nil
}

// mcpOptions guards against a classic Go nil-interface trap: Runtime.Recorder is a concrete
// *hotel.Recorder that is legally nil when OTel is disabled. Assigning that nil pointer straight
// into an interface-typed field produces a non-nil interface wrapping a nil value, so every
// downstream `recorder != nil` check passes and the nil receiver's method panics on first use.
// Only assign the field when the concrete pointer is actually non-nil.
func mcpOptions(otelRuntime loomotel.Runtime, exportRoot string) loommcp.Options {
	opts := loommcp.Options{ExportRoot: exportRoot}
	if otelRuntime.Recorder != nil {
		opts.Recorder = otelRuntime.Recorder
	}
	return opts
}

// configureLLMProvider wires an Anthropic-backed llm.Provider into comp when
// loom.llm_api_key_ref names a secret, so compile jobs with generation_mode=llm work. With no
// reference it is a no-op and such jobs fail with a clear error from the compiler; the
// deterministic path never depends on this. The loom.redact_patterns are applied to job bodies
// before they are sent to the provider, in addition to internal/llm's baseline redactors.
func configureLLMProvider(ctx context.Context, app *module.App, comp *service.Compiler, cfg settings.Settings) error {
	if cfg.LLMAPIKeyRef == "" {
		return nil
	}
	resolver := secrets.Resolver(app)
	if resolver == nil {
		return errors.New("loom.llm_api_key_ref is set but the secrets module is not started")
	}
	res, err := resolver.Resolve(ctx, cfg.LLMAPIKeyRef)
	if err != nil {
		return fmt.Errorf("loom.llm_api_key_ref: %w", err)
	}
	provider, err := loomanthropic.New(res.Secret.Reveal(), cfg.LLMModel)
	if err != nil {
		return err
	}
	comp.SetLLMProvider(provider, cfg.RedactPatterns)
	return nil
}

func (l *loom) serveCmd(ctx context.Context) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Serve the API, the MCP endpoint and the Sysop UI",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return l.serve(ctx) },
	}
}

func (l *loom) mcpCmd(ctx context.Context) *cobra.Command {
	return &cobra.Command{
		Use:                "mcp",
		Short:              "Serve the MCP tools over stdio (needs mcp.stdio_token_file in enforce mode)",
		DisableFlagParsing: true,
		RunE: func(*cobra.Command, []string) error {
			defer l.otel.Shutdown()
			return mcp.ServeStdio(ctx, l.app, mcp.StdioOptions{})
		},
	}
}

// serve runs the HTTP host until ctx ends.
func (l *loom) serve(ctx context.Context) error {
	app := l.app
	defer l.otel.Shutdown()
	id := identity.From(app)
	if id == nil {
		return errors.New("the identity module must be started")
	}
	api.RegisterScopes(auth.ScopeTable(app))
	assets, err := webui.Assets()
	if err != nil {
		return err
	}
	handler, err := httphost.Handler(app, httphost.Options{
		Assets: assets,
		Routes: api.New(l.repo, l.comp).WithExportRoot(l.settings.ExportDir).Mux(),
	})
	if err != nil {
		return err
	}
	// Outermost first: metrics and the body cap, then the Host check (DNS rebinding) and
	// forwarding-header hygiene, then the session cookie becomes the bearer the guard verifies,
	// then the host (guard, routes, SPA).
	handler = loomserver.Wrap(hostguard.Handler(l.settings.AllowedHosts, id.Middleware(handler)), serverConfig(l.otel))
	return httphost.Serve(ctx, app, handler, httphost.ServeOptions{
		Ready: func(addr net.Addr) {
			slog.Info("loom listening", "addr", addr.String())
			if l.ready != nil {
				l.ready(addr)
			}
		},
	})
}

func serverConfig(otelRuntime loomotel.Runtime) loomserver.Config {
	cfg := loomserver.Config{RouteResolver: loomRoute}
	if otelRuntime.Recorder != nil {
		cfg.Recorder = otelRuntime.Recorder
	}
	return cfg
}

// commands are the flag-based local commands. They work on the same database as the server, with
// no HTTP and so no sign-in: whoever can open the file can run them.
func (l *loom) commands(ctx context.Context) map[string]func(args []string) error {
	return map[string]func(args []string) error{
		"compile":   func(a []string) error { return l.compileCLI(ctx, a) },
		"jobs":      func(a []string) error { return l.jobsCLI(ctx, a) },
		"bundles":   func(a []string) error { return l.bundlesCLI(ctx, a) },
		"templates": func(a []string) error { return l.templatesCLI(ctx, a) },
		"pages":     func(a []string) error { return l.pagesCLI(ctx, a) },
		"ingest":    func(a []string) error { return l.ingestCLI(ctx, a) },
		"export":    func(a []string) error { return l.exportCLI(ctx, a) },
		"directives": func(a []string) error {
			if len(a) > 0 && a[0] == "parse" {
				return directivesCLI(a[1:])
			}
			return l.directivesLedgerCLI(ctx, a)
		},
		"path":   pathCLI,
		"status": func([]string) error { return l.statusCLI(ctx) },
	}
}
