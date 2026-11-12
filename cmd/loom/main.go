// Command loom is the Loom wiki compiler, built on Substrate. The shell (config, logging, health,
// HTTP, auth, identity, secrets, database, MCP plumbing, CLI skeleton) comes from the manifest;
// this package and internal/ are the app: its schema, its routes, its tools and its commands.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/hollis-labs/substrate/modules/cli"
)

const version = "0.1.0"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "loom:", err)
		stop()
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	// -db, -workspace and -project/--no-project choose the data directory, which the modules need
	// before they start; the commands get the rest.
	rest, appArgs, err := splitAppArgs(args)
	if err != nil {
		return err
	}
	modules, kind := plan(rest)
	// Only the commands that write create the data directories; the rest resolve the layout
	// without creating anything.
	migrate := len(rest) > 0 && rest[0] == "migrate-data"
	layout, err := layoutFor(appArgs, (kind == kindFull || kind == kindIdentity) && !migrate)
	if err != nil {
		return err
	}
	if migrate {
		// Runs before the modules start: it builds the new database itself, and an old
		// database would make them refuse to start.
		return migrateData(ctx, layout, explicitDB(appArgs), rest[1:], os.Stdout)
	}
	started, err := startModulesFor(ctx, layout, explicitDB(appArgs), modules, kind == kindStatus)
	var none *noDatabaseError
	if errors.As(err, &none) {
		// status, before any database exists: say so; nothing was created.
		return noDatabaseStatus(none.path, os.Stdout)
	}
	if err != nil {
		return err
	}
	defer func() { _ = started.Stop(context.Background()) }()

	var l *loom
	switch kind {
	case kindFull:
		l, err = newLoom(ctx, started.App, layout)
		if err != nil {
			return err
		}
	case kindStatus:
		l = readerLoom(started.App)
	default:
		// The commands are registered so that help lists them; none of them can run in this mode
		// (plan sent them to the full set), so the shell needs nothing behind it.
		l = &loom{app: started.App}
	}
	l.layout = layout
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
