// Package storage owns Loom's durable schema and the repository over it. The schema is one
// embedded goose migration applied by Substrate's database module. Loom is SQLite only.
package storage

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"

	_ "modernc.org/sqlite" // registers the "sqlite" driver the legacy guard uses
)

//go:embed migrations
var migrations embed.FS

// Migrations is the embedded migration tree: root "migrations" holds one directory per
// dialect (here only sqlite).
func Migrations() (fsys embed.FS, root string) { return migrations, "migrations" }

// ErrLegacyLayout means the database file holds the layout used before Loom moved onto
// Substrate. `loom migrate-data` copies it into the new schema (see legacy.go).
var ErrLegacyLayout = errors.New("the database uses the pre-Substrate Loom layout")

// RefuseLegacy returns ErrLegacyLayout (wrapped with what to do) when the SQLite file at
// path already exists with the old layout: a schema_version table, which the old
// hand-rolled migrator kept and the goose one never creates. Opening such a file would add the
// new tables beside the old ones, so it is refused. A missing or empty file is fine. Run it before the database module opens the file, because opening migrates.
func RefuseLegacy(path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	// Only reads: the guard never changes the file it is judging.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	var legacy int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='schema_version'").Scan(&legacy); err != nil {
		return fmt.Errorf("inspect %s: %w", path, err)
	}
	if legacy > 0 {
		return fmt.Errorf("%w (%s): run `loom migrate-data --from <old> --to <new>` and open the new file. See docs/conversion-notes.md",
			ErrLegacyLayout, path)
	}
	return nil
}
