package main

import (
	"context"
	"fmt"
	"path/filepath"

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
func startModules(ctx context.Context, layout paths.Layout, explicitDB bool) (*boot.Running, error) {
	useMigrations()
	return boot.StartWith(ctx, substrateApp, substrateModules, substrateConfigJSON, boot.Options{
		AfterConfig: func(_ context.Context, app *module.App) error {
			resolvePaths(app, layout, explicitDB)
			return nil
		},
		Before: map[string]boot.Hook{
			"database": func(_ context.Context, app *module.App) error { return guardLegacy(app, layout) },
		},
	})
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
