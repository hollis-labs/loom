package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/hollis-labs/libs/core/store"
	"github.com/hollis-labs/libs/core/store/migrate"
)

// legacyVersions are the migrations the pre-Substrate migrator recorded in schema_version. A
// database that has all of them has the final old schema, which the new one is column-for-column.
var legacyVersions = []string{
	"001_wiki_core", "002_compile_jobs", "003_directives_templates", "004_ingest_ledger",
	"005_default_template_body", "006_wiki_bundle_page_okf_fields", "007_wiki_links_redesign",
	"008_wiki_verifications_actor", "009_wiki_result_cache",
}

// copyOrder is the tables copied, parents before children. wiki_result_cache is deliberately
// not copied: it is a TTL cache keyed by caller-supplied ids that no longer mean anything.
var copyOrder = []string{
	"wiki_bundles", "wiki_pages", "wiki_links", "wiki_verifications",
	"compile_jobs", "compile_events", "directive_ledger", "templates", "ingest_ledger",
}

// NewPath is the database file the Substrate-era Loom opens for a workspace whose pre-Substrate
// file is old: a sibling named loom.db, so the old main.db stays where it is, untouched, as the
// rollback.
func NewPath(old string) string { return filepath.Join(filepath.Dir(old), "loom.db") }

// RequireMigrated refuses to start on a data directory whose old file holds data and whose new
// file does not exist yet, which would silently show the user an empty Loom.
func RequireMigrated(old, current string) error {
	if old == current {
		return nil // an explicit database path: RefuseLegacy judges the file itself
	}
	if _, err := os.Stat(current); err == nil {
		return nil
	}
	legacy, err := hasLegacyLayout(old)
	if err != nil {
		return err
	}
	if legacy {
		return fmt.Errorf("%w (%s) and no migrated database exists at %s: run `loom migrate-data` (see docs/conversion-notes.md)",
			ErrLegacyLayout, old, current)
	}
	return nil
}

// MigrateReport is what a migration measured.
type MigrateReport struct {
	From, To string
	Counts   map[string]int
	Duration time.Duration
}

// MigrateLegacy copies every row of the pre-Substrate database at from into a new database at
// to, with keys preserved. It reads from read-only, never writes to from, builds to under a
// temporary name and only renames it into place after the row counts match and
// integrity_check and foreign_key_check are clean, so a failed run leaves nothing at to. It
// refuses when to already exists, so a second run cannot overwrite migrated data.
func MigrateLegacy(ctx context.Context, from, to string) (MigrateReport, error) {
	start := time.Now()
	rep := MigrateReport{From: from, To: to, Counts: map[string]int{}}
	if from == to {
		return rep, errors.New("migrate: from and to are the same file")
	}
	if _, err := os.Stat(from); err != nil {
		return rep, fmt.Errorf("migrate: old database: %w", err)
	}
	if _, err := os.Stat(to); err == nil {
		return rep, fmt.Errorf("migrate: %s already exists; refusing to overwrite (this file was already migrated; choose another --to to migrate again)", to)
	}
	if err := requireFinalLegacySchema(ctx, from); err != nil {
		return rep, err
	}

	tmp := to + ".migrating"
	for _, f := range []string{tmp, tmp + "-wal", tmp + "-shm"} {
		_ = os.Remove(f)
	}
	cfg := store.Config{Driver: store.DriverSQLite, SQLite: store.SQLiteConfig{Path: tmp, CreateParentDir: true}}
	st, err := store.Open(ctx, cfg)
	if err != nil {
		return rep, fmt.Errorf("migrate: create new database: %w", err)
	}
	cleanup := func() {
		_ = st.Close()
		for _, f := range []string{tmp, tmp + "-wal", tmp + "-shm"} {
			_ = os.Remove(f)
		}
	}
	fsys, root := Migrations()
	if err = migrate.Ensure(ctx, cfg, st, migrate.Source{FS: fsys, Root: root}); err != nil {
		cleanup()
		return rep, fmt.Errorf("migrate: schema: %w", err)
	}
	if err = copyTables(ctx, st.DB(), from); err != nil {
		cleanup()
		return rep, err
	}
	if err = verify(ctx, st.DB(), from, rep.Counts); err != nil {
		cleanup()
		return rep, err
	}
	// Fold the WAL into the file so the rename moves one complete database.
	if _, err = st.DB().ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		cleanup()
		return rep, fmt.Errorf("migrate: checkpoint: %w", err)
	}
	if err = st.Close(); err != nil {
		cleanup()
		return rep, err
	}
	for _, f := range []string{tmp + "-wal", tmp + "-shm"} {
		_ = os.Remove(f)
	}
	if err = os.Rename(tmp, to); err != nil {
		_ = os.Remove(tmp)
		return rep, fmt.Errorf("migrate: install new database: %w", err)
	}
	rep.Duration = time.Since(start)
	return rep, nil
}

func hasLegacyLayout(path string) (bool, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	db, err := openReadOnly(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='schema_version'").Scan(&n); err != nil {
		return false, fmt.Errorf("inspect %s: %w", path, err)
	}
	return n > 0, nil
}

func openReadOnly(path string) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
	return sql.Open("sqlite", u.String())
}

func requireFinalLegacySchema(ctx context.Context, from string) error {
	db, err := openReadOnly(from)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_version`)
	if err != nil {
		return fmt.Errorf("migrate: %s is not a pre-Substrate Loom database (no schema_version): %w", from, err)
	}
	defer func() { _ = rows.Close() }()
	have := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return err
		}
		have[v] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, v := range legacyVersions {
		if !have[v] {
			return fmt.Errorf("migrate: %s is missing the old migration %s; open it once with the previous Loom build to finish its migrations, then run again", from, v)
		}
	}
	return nil
}

func tableColumns(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, schema, table string) ([]string, error) {
	//nolint:gosec // table is from the fixed copyOrder list
	rows, err := q.QueryContext(ctx, fmt.Sprintf(`SELECT name FROM pragma_table_info('%s', '%s')`, table, schema))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var cols []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

func quoteIdent(s string) string { return `"` + s + `"` }

// copyTables attaches the old file read-only and copies each table by column name in one
// transaction. The new schema's seed rows (the nanite bundle, the default template) are removed
// first: the old data is authoritative, including a deleted seed.
func copyTables(ctx context.Context, db *sql.DB, from string) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	u := url.URL{Scheme: "file", Path: from, RawQuery: "mode=ro"}
	if _, err = conn.ExecContext(ctx, `ATTACH DATABASE ? AS old`, u.String()); err != nil {
		return fmt.Errorf("migrate: attach old database: %w", err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), `DETACH DATABASE old`) }()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, t := range []string{"templates", "wiki_bundles"} {
		//nolint:gosec // t is from a fixed list
		if _, err = tx.ExecContext(ctx, `DELETE FROM main.`+quoteIdent(t)); err != nil {
			return fmt.Errorf("migrate: clear seed rows of %s: %w", t, err)
		}
	}
	for _, t := range copyOrder {
		cols, err := tableColumns(ctx, tx, "main", t)
		if err != nil {
			return fmt.Errorf("migrate: columns of new table %s: %w", t, err)
		}
		if len(cols) == 0 {
			return fmt.Errorf("migrate: new table %s has no columns", t)
		}
		list := ""
		for i, c := range cols {
			if i > 0 {
				list += ", "
			}
			list += quoteIdent(c)
		}
		//nolint:gosec // table and column names come from the fixed copyOrder list and the new schema
		q := fmt.Sprintf(`INSERT INTO main.%s (%s) SELECT %s FROM old.%s`, quoteIdent(t), list, list, quoteIdent(t))
		if _, err = tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("migrate: copy %s: %w", t, err)
		}
	}
	return tx.Commit()
}

// verify compares per-table row counts between the old and new files, and runs the new file's
// integrity_check and foreign_key_check.
func verify(ctx context.Context, db *sql.DB, from string, counts map[string]int) error {
	old, err := openReadOnly(from)
	if err != nil {
		return err
	}
	defer func() { _ = old.Close() }()
	for _, t := range copyOrder {
		var before, after int
		//nolint:gosec // t is from the fixed copyOrder list
		if err = old.QueryRowContext(ctx, `SELECT count(*) FROM `+quoteIdent(t)).Scan(&before); err != nil {
			return fmt.Errorf("migrate: count old %s: %w", t, err)
		}
		//nolint:gosec // t is from the fixed copyOrder list
		if err = db.QueryRowContext(ctx, `SELECT count(*) FROM `+quoteIdent(t)).Scan(&after); err != nil {
			return fmt.Errorf("migrate: count new %s: %w", t, err)
		}
		if before != after {
			return fmt.Errorf("migrate: %s has %d rows in the old database and %d in the new one", t, before, after)
		}
		counts[t] = after
	}
	var integrity string
	if err = db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil {
		return fmt.Errorf("migrate: integrity_check of the new database: %w", err)
	}
	if integrity != "ok" {
		return fmt.Errorf("migrate: integrity_check of the new database: %q", integrity)
	}
	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		return errors.New("migrate: foreign_key_check of the new database reports violations")
	}
	return rows.Err()
}
