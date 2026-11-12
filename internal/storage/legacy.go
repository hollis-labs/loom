package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	before, err := fingerprint(from)
	if err != nil {
		cleanup()
		return rep, err
	}
	if err = copyTables(ctx, st.DB(), from); err != nil {
		cleanup()
		return rep, err
	}
	if err = verify(ctx, st.DB(), from, rep.Counts); err != nil {
		cleanup()
		return rep, err
	}
	if afterChecksHook != nil {
		afterChecksHook()
	}
	// The old file is opened without locks when it has no WAL content (see readOnlyURI), so a
	// writer running on it could be read mid-change and the content check would compare the
	// copy with the same torn read. If the old files are not exactly as they were before the
	// copy, nothing is installed.
	after, err := fingerprint(from)
	if err != nil {
		cleanup()
		return rep, err
	}
	if after != before {
		cleanup()
		return rep, errors.New("migrate: the old database changed during the migration; stop whatever is writing to it and run again")
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

// afterChecksHook is called between the checks and the second fingerprint; tests use it to write
// to the old file at that moment.
var afterChecksHook func()

// fingerprint is the size, modification time and sha256 of the old database and of its -wal and
// -journal files (absent files count as absent). The files here are small, so hashing costs
// nothing; an app with a large file would compare size and modification time only.
func fingerprint(path string) (string, error) {
	var b strings.Builder
	for _, f := range []string{path, path + "-wal", path + "-journal"} {
		info, err := os.Stat(f)
		if errors.Is(err, os.ErrNotExist) {
			b.WriteString(f + ": absent\n")
			continue
		}
		if err != nil {
			return "", fmt.Errorf("migrate: inspect %s: %w", f, err)
		}
		fh, err := os.Open(f) //nolint:gosec // the operator names the old database
		if err != nil {
			return "", fmt.Errorf("migrate: read %s: %w", f, err)
		}
		h := sha256.New()
		_, cerr := io.Copy(h, fh)
		_ = fh.Close()
		if cerr != nil {
			return "", fmt.Errorf("migrate: read %s: %w", f, cerr)
		}
		fmt.Fprintf(&b, "%s: %d %d %x\n", f, info.Size(), info.ModTime().UnixNano(), h.Sum(nil))
	}
	return b.String(), nil
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
	return sql.Open("sqlite", readOnlyURI(path))
}

// readOnlyURI is how the old file is opened, to judge it and to copy from it. mode=ro alone is
// not enough: on a WAL-mode file SQLite still creates a -shm (and -wal) beside it, and the old
// file must be left exactly as found. When there is no WAL content to read (no -wal, or an empty
// one, which is what a cleanly stopped writer leaves) the file is opened immutable, which takes
// no locks and writes nothing; when a -wal with content exists it is opened mode=ro so that
// content is still read (the files are already there, so none is created), waiting up to 5s for a
// live writer rather than failing.
func readOnlyURI(path string) string {
	// A live writer (the other process starting on the same file) can hold the WAL in recovery
	// for a moment; wait for it rather than failing the start (SQLITE_BUSY_RECOVERY).
	q := "mode=ro&_pragma=busy_timeout(5000)"
	if fi, err := os.Stat(path + "-wal"); err != nil || fi.Size() == 0 {
		q = "mode=ro&immutable=1"
	}
	return (&url.URL{Scheme: "file", Path: path, RawQuery: q}).String()
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
	if _, err = conn.ExecContext(ctx, `ATTACH DATABASE ? AS old`, readOnlyURI(from)); err != nil {
		return fmt.Errorf("migrate: attach old database: %w", err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), `DETACH DATABASE old`) }()
	if err = requireKnownObjects(ctx, conn); err != nil {
		return err
	}
	if err = requireSameColumns(ctx, conn); err != nil {
		return err
	}
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
		if err = requireSameContent(ctx, tx, t, list); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// requireKnownObjects refuses an old file that holds anything the copy does not know about: a
// table outside copyOrder (other than the deliberately skipped cache, schema_version and SQLite's
// own), a trigger, a view, a virtual table, or a table that uses AUTOINCREMENT (whose sequence
// would not carry over). The pre-Substrate schema (the nine numbered migrations) had none of the
// last three.
func requireKnownObjects(ctx context.Context, conn *sql.Conn) error {
	known := map[string]bool{"wiki_result_cache": true, "schema_version": true}
	for _, t := range copyOrder {
		known[t] = true
	}
	rows, err := conn.QueryContext(ctx, `SELECT type, name, COALESCE(sql, '') FROM old.sqlite_master WHERE name NOT LIKE 'sqlite\_%' ESCAPE '\'`)
	if err != nil {
		return fmt.Errorf("migrate: list the old database's objects: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var typ, name, ddl string
		if err = rows.Scan(&typ, &name, &ddl); err != nil {
			return err
		}
		upper := strings.ToUpper(ddl)
		switch {
		case typ == "trigger" || typ == "view":
			return fmt.Errorf("migrate: the old database has a %s %q that the copy would drop; stopping", typ, name)
		case typ == "table" && strings.Contains(upper, "CREATE VIRTUAL TABLE"):
			return fmt.Errorf("migrate: the old database has a virtual table %q that the copy would drop; stopping", name)
		case typ == "table" && strings.Contains(upper, "AUTOINCREMENT"):
			return fmt.Errorf("migrate: the old table %q uses AUTOINCREMENT, whose sequence the copy does not carry; stopping", name)
		case typ == "table" && !known[name] && !strings.HasPrefix(name, "sqlite_"):
			return fmt.Errorf("migrate: the old database has a table %q the migration does not know; stopping rather than dropping it", name)
		}
	}
	return rows.Err()
}

// columnInfo is what pragma_table_info reports for one column.
type columnInfo struct {
	Type    string
	NotNull int
	PK      int
}

func schemaColumns(ctx context.Context, conn *sql.Conn, schema, table string) (map[string]columnInfo, error) {
	//nolint:gosec // table and schema come from the fixed copyOrder list
	rows, err := conn.QueryContext(ctx, fmt.Sprintf(`SELECT name, type, "notnull", pk FROM pragma_table_info('%s', '%s')`, table, schema))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]columnInfo{}
	for rows.Next() {
		var name string
		var c columnInfo
		if err = rows.Scan(&name, &c.Type, &c.NotNull, &c.PK); err != nil {
			return nil, err
		}
		out[name] = c
	}
	return out, rows.Err()
}

// requireSameColumns compares each copied table's columns in the old and new schemas: name,
// declared type, NOT NULL and primary-key position. The copy selects the new table's columns
// from the old one, so a column only the old table has would be dropped without a word.
func requireSameColumns(ctx context.Context, conn *sql.Conn) error {
	for _, t := range copyOrder {
		oldCols, err := schemaColumns(ctx, conn, "old", t)
		if err != nil {
			return fmt.Errorf("migrate: columns of old table %s: %w", t, err)
		}
		newCols, err := schemaColumns(ctx, conn, "main", t)
		if err != nil {
			return fmt.Errorf("migrate: columns of new table %s: %w", t, err)
		}
		if len(oldCols) == 0 {
			return fmt.Errorf("migrate: the old database has no table %s", t)
		}
		for name, o := range oldCols {
			n, ok := newCols[name]
			if !ok {
				return fmt.Errorf("migrate: old column %s.%s has no counterpart in the new schema; stopping rather than dropping it", t, name)
			}
			if n != o {
				return fmt.Errorf("migrate: column %s.%s differs: old %+v, new %+v", t, name, o, n)
			}
		}
		for name := range newCols {
			if _, ok := oldCols[name]; !ok {
				return fmt.Errorf("migrate: new column %s.%s does not exist in the old table", t, name)
			}
		}
	}
	return nil
}

// requireSameContent proves the copy by content, inside the copy transaction: no row of the old
// table is missing or changed in the new one, and the new one has no row the old one lacks. A
// matching row count with a truncated or altered value fails here.
func requireSameContent(ctx context.Context, tx *sql.Tx, table, cols string) error {
	q := quoteIdent(table)
	for _, dir := range []struct{ a, b, what string }{
		{"old", "main", "in the old database but missing or changed in the new one"},
		{"main", "old", "in the new database but not in the old one"},
	} {
		var n int
		//nolint:gosec // table and column list come from the fixed copyOrder list and the new schema
		query := fmt.Sprintf(`SELECT count(*) FROM (SELECT %s FROM %s.%s EXCEPT SELECT %s FROM %s.%s)`, cols, dir.a, q, cols, dir.b, q)
		if err := tx.QueryRowContext(ctx, query).Scan(&n); err != nil {
			return fmt.Errorf("migrate: compare %s content: %w", table, err)
		}
		if n != 0 {
			return fmt.Errorf("migrate: %d rows of %s are %s", n, table, dir.what)
		}
	}
	return nil
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
