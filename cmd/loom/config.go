package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	paths "github.com/hollis-labs/libs/core/util/apppaths"

	"github.com/hollis-labs/loom/internal/storage"
)

// prepare returns the manifest config (the generated substrateConfigJSON) with the files that
// live in the data directory as absolute paths. The manifest holds working-directory defaults;
// the data directory (project-local .loom/, the -db/-workspace options, XDG) is only known at
// run time.
//
// It also refuses a database that still has the pre-Substrate layout, before the database
// module opens (and so migrates) the file.
func prepare(layout paths.Layout, overrideFile string, explicitDB bool) (string, error) {
	cfg := map[string]any{}
	if err := json.Unmarshal([]byte(substrateConfigJSON), &cfg); err != nil {
		return "", err
	}
	set := func(module, key, value string, nested ...string) {
		m, _ := cfg[module].(map[string]any)
		if m == nil {
			m = map[string]any{}
			cfg[module] = m
		}
		for _, n := range nested {
			child, _ := m[n].(map[string]any)
			if child == nil {
				child = map[string]any{}
				m[n] = child
			}
			m = child
		}
		m[key] = value
	}
	// The Substrate-era database is loom.db beside the old main.db, which stays untouched as the
	// rollback (see `loom migrate-data`). An explicit -db is taken literally.
	dbFile := databasePath(layout, explicitDB)
	if err := storage.RequireMigrated(layout.MainDB(), dbFile); err != nil {
		return "", err
	}
	set("database", "path", dbFile, "sqlite")
	// Identity and secrets state live beside the database actually opened. With an explicit -db,
	// layout.DataDir() is still the XDG default, so using it would write into another
	// installation's directory.
	stateDir := layout.DataDir()
	if explicitDB {
		stateDir = filepath.Dir(dbFile)
	}
	set("identity", "key_file", filepath.Join(stateDir, "identity-signing.key"))
	set("secrets", "state_dir", filepath.Join(stateDir, "secrets"))

	dbPath, err := effectiveDatabasePath(cfg, overrideFile)
	if err != nil {
		return "", err
	}
	if err = storage.RefuseLegacy(dbPath); err != nil {
		return "", err
	}
	out, err := json.Marshal(cfg)
	return string(out), err
}

// effectiveDatabasePath is the SQLite file the database module will open: the config's path,
// unless the SUBSTRATE_CONFIG file replaces database.sqlite (the config module merges one
// level deep, so a replaced sqlite block without a path falls back to the module default).
func effectiveDatabasePath(cfg map[string]any, overrideFile string) (string, error) {
	path := func(c map[string]any) (string, bool) {
		db, _ := c["database"].(map[string]any)
		sq, ok := db["sqlite"].(map[string]any)
		if !ok {
			return "", false
		}
		p, _ := sq["path"].(string)
		return p, true
	}
	p, _ := path(cfg)
	if overrideFile != "" {
		b, err := os.ReadFile(overrideFile) //nolint:gosec // the operator names this file
		if err != nil {
			return "", fmt.Errorf("read %s: %w", overrideFile, err)
		}
		var override map[string]any
		if err := yaml.Unmarshal(b, &override); err != nil {
			return "", fmt.Errorf("parse %s: %w", overrideFile, err)
		}
		if op, replaced := path(override); replaced {
			p = op
			if p == "" {
				p = substrateApp + ".db" // the database module's default, relative to the working directory
			}
		}
	}
	if p == "" {
		return "", fmt.Errorf("no database path configured")
	}
	return filepath.Clean(p), nil
}

// databasePath is the file the database module opens for layout.
func databasePath(layout paths.Layout, explicitDB bool) string {
	if explicitDB {
		return layout.MainDB()
	}
	return storage.NewPath(layout.MainDB())
}
