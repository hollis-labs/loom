// Command loom is the Loom wiki compiler, built on Substrate. The shell (config, logging, health,
// HTTP, auth, identity, secrets, database, MCP plumbing, CLI skeleton) comes from the manifest;
// this package and internal/ are the app: its schema, its routes, its tools and its commands.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/hollis-labs/substrate/boot"
	"github.com/hollis-labs/substrate/modules/cli"
	"github.com/hollis-labs/substrate/modules/database"

	"github.com/hollis-labs/loom/internal/storage"
)

const version = "0.1.0"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "loom:", err)
		stop()
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string) error {
	// -db, -workspace and -project/--no-project choose the data directory, which the modules need
	// before they start; the commands get the rest.
	rest, appArgs, err := splitAppArgs(args)
	if err != nil {
		return err
	}
	layout, err := resolveLayout(appArgs)
	if err != nil {
		return err
	}
	if len(rest) > 0 && rest[0] == "migrate-data" {
		// Runs before the modules start: it builds the new database itself, and an old
		// database would make them refuse to start.
		return migrateData(ctx, layout, explicitDB(appArgs), rest[1:], os.Stdout)
	}
	config, err := prepare(layout, getenv("SUBSTRATE_CONFIG"), explicitDB(appArgs))
	if err != nil {
		return err
	}
	useMigrations()

	started, err := boot.Start(ctx, substrateApp, substrateModules, config)
	if err != nil {
		return err
	}
	defer func() { _ = started.Stop(context.Background()) }()

	l, err := newLoom(ctx, started.App, layout)
	if err != nil {
		return err
	}
	rootCmd := cli.Root(started.App)
	serve := l.serveCmd(ctx)
	// `loom` with no command serves, like `loom serve`.
	rootCmd.RunE = func(c *cobra.Command, args []string) error { return serve.RunE(c, args) }
	rootCmd.Args = cobra.NoArgs
	rootCmd.AddCommand(serve, l.mcpCmd(ctx))
	for name, run := range l.commands(ctx) {
		rootCmd.AddCommand(passthrough(name, run))
	}
	rootCmd.SetArgs(rest)
	return rootCmd.ExecuteContext(ctx)
}

// passthrough wraps a flag-based command: cobra hands it the raw arguments (the data options are
// already applied and removed).
func passthrough(name string, run func(args []string) error) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		Short:              "loom " + name,
		DisableFlagParsing: true,
		RunE:               func(_ *cobra.Command, args []string) error { return run(args) },
	}
}

// useMigrations installs the embedded schema as the database module's migration source. It
// must run before boot.Start.
func useMigrations() {
	fsys, root := storage.Migrations()
	database.UseMigrationFS(fsys, root)
}
