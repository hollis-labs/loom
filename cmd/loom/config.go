package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	paths "github.com/hollis-labs/libs/core/util/apppaths"

	"github.com/hollis-labs/substrate/boot"
	"github.com/hollis-labs/substrate/module"
	"github.com/hollis-labs/substrate/modules/database"

	"github.com/hollis-labs/loom/internal/storage"
)

// startModules starts the modules the manifest selects. The data directory (project-local
// .loom/, the -db/-workspace options, XDG) is only known at run time, so two boot hooks use it:
// AfterConfig makes the file paths absolute, and a guard runs before the database module opens
// (and so migrates) the file.
func startModules(ctx context.Context, layout paths.Layout) (*boot.Running, error) {
	return startModulesFor(ctx, layout, false, substrateModules, false)
}

// startModulesFor starts the named modules (in manifest order). With existingOnly the database
// module is started only for a database file that already exists and is migrated; for any other
// state it stops before the file is touched and returns errNoDatabase (the pre-Substrate layout
// is still refused by the guard, naming loom migrate-data).
func startModulesFor(ctx context.Context, layout paths.Layout, explicitDB bool, modules []string, existingOnly bool) (*boot.Running, error) {
	useMigrations()
	before := map[string]boot.Hook{}
	if slices.Contains(modules, "database") { // a hook for a module that is not started is an error
		before["database"] = func(_ context.Context, app *module.App) error {
			if err := guardLegacy(app, layout); err != nil {
				return err
			}
			if existingOnly {
				if path := databasePath(app); path != "" {
					if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
						return &noDatabaseError{path: path}
					}
				}
			}
			return nil
		}
	}
	return boot.StartWith(ctx, substrateApp, modules, substrateConfigJSON, boot.Options{
		AfterConfig: func(_ context.Context, app *module.App) error {
			resolvePaths(app, layout, explicitDB)
			return nil
		},
		Before: before,
	})
}

// noDatabaseError is what a read-only command that needs the database gets when there is none yet:
// not a failure to report with a stack, but a state to describe, with nothing created.
type noDatabaseError struct{ path string }

func (e *noDatabaseError) Error() string { return "no database yet at " + e.path }

// databasePath is the sqlite path the database module is about to open.
func databasePath(app *module.App) string {
	db, _ := app.Config["database"].(map[string]any)
	sq, _ := db["sqlite"].(map[string]any)
	path, _ := sq["path"].(string)
	return path
}

// Command kinds, from what a command may create. Only the first two kinds write anything.
const (
	kindFull     = iota // serve (the default), mcp and the commands that write data: every module
	kindIdentity        // identity setup, identity key create: the database, the signing key, an owner or a key; no listener
	kindStatus          // status: the database only if it already exists and is migrated
	kindReadOnly        // help, version, config, health, path, unknown words: no module that opens or creates a file
)

// The module sets, in manifest order. A module that opens or creates a file (database, secrets,
// identity, the signing key) is in none of the read-only sets.
var (
	readOnlyModules = []string{"config", "logging", "health", "cli"}
	statusModules   = []string{"config", "logging", "database", "cli"}
	// the database, the identity module and the auth module it requires; secrets because the boot
	// hook places its directory.
	identityModules = []string{"config", "logging", "secrets", "auth", "database", "identity", "cli"}
)

// dataCommands write to, or are bound to, the data: they may create the database and the key
// like serve does. (bundles, pages and the rest also have read subcommands; they are listed whole.)
var dataCommands = []string{"mcp", "compile", "jobs", "bundles", "templates", "pages", "ingest", "export", "directives"}

// plan decides, from the command line (after the data options are removed), which modules to start.
// Every argument that does not start with "-" counts as a command word. That is right only while
// the root command has no global flag that takes a value (cobra's -h/--help is the one flag):
// `--level debug serve` would read "debug" as the command. TestRootHasNoValueFlags fails when
// such a flag is added, and then plan must learn it.
func plan(args []string) (modules []string, kind int) {
	var words []string
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "--version" {
			return pickModules(readOnlyModules), kindReadOnly
		}
		if !strings.HasPrefix(a, "-") {
			words = append(words, a)
		}
	}
	switch {
	case len(words) == 0 || words[0] == "serve" || slices.Contains(dataCommands, words[0]):
		return substrateModules, kindFull
	case words[0] == "identity" && len(words) > 1 && (words[1] == "setup" || (words[1] == "key" && len(words) > 2 && words[2] == "create")):
		return pickModules(identityModules), kindIdentity
	case words[0] == "status":
		return pickModules(statusModules), kindStatus
	}
	return pickModules(readOnlyModules), kindReadOnly
}

// pickModules keeps the start order of substrateModules.
func pickModules(want []string) []string {
	var out []string
	for _, m := range substrateModules {
		if slices.Contains(want, m) {
			out = append(out, m)
		}
	}
	return out
}

// resolvePaths writes the absolute locations of the files that live in the data directory into
// app.Config, after the SUBSTRATE_CONFIG override is applied. A path the operator's override
// already made absolute is left alone; a relative one is placed in the data directory.
//
// The Substrate-era database is loom.db beside the old main.db, which stays untouched as the
// rollback (see `loom migrate-data`). An explicit -db is taken literally, and the identity key
// and secrets state then sit beside that file: layout.DataDir() is still the XDG default, so
// using it would write into another installation's directory.
func resolvePaths(app *module.App, layout paths.Layout, explicitDB bool) {
	dbDir := filepath.Dir(layout.MainDB())
	dataDir := layout.DataDir()
	if explicitDB {
		dataDir = dbDir
	}
	if explicitDB {
		setConfig(app, layout.MainDB(), "database", "sqlite", "path")
	} else {
		place(app, dbDir, "database", "sqlite", "path")
	}
	place(app, dataDir, "identity", "key_file")
	place(app, dataDir, "secrets", "state_dir")
}

// place makes the config value at path absolute under dir, unless it already is.
func place(app *module.App, dir string, path ...string) {
	m := configMap(app, path[:len(path)-1])
	key := path[len(path)-1]
	if p, _ := m[key].(string); !filepath.IsAbs(p) {
		m[key] = filepath.Join(dir, p)
	}
}

func setConfig(app *module.App, value string, path ...string) {
	configMap(app, path[:len(path)-1])[path[len(path)-1]] = value
}

// configMap returns the nested map at path, creating the maps on the way.
func configMap(app *module.App, path []string) map[string]any {
	m := app.Config
	for _, k := range path {
		child, _ := m[k].(map[string]any)
		if child == nil {
			child = map[string]any{}
			m[k] = child
		}
		m = child
	}
	return m
}

// guardLegacy refuses a database that still has the pre-Substrate layout, reading the path the
// database module is about to open from app.Config.
func guardLegacy(app *module.App, layout paths.Layout) error {
	db, _ := app.Config["database"].(map[string]any)
	sq, _ := db["sqlite"].(map[string]any)
	path, _ := sq["path"].(string)
	if path == "" {
		return fmt.Errorf("no database path configured")
	}
	if err := storage.RequireMigrated(layout.MainDB(), path); err != nil {
		return err
	}
	return storage.RefuseLegacy(path)
}

// useMigrations installs the embedded schema as the database module's migration source. It
// must run before boot.
func useMigrations() {
	fsys, root := storage.Migrations()
	database.UseMigrationFS(fsys, root)
}
