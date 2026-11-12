package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// requireSameContent must catch what a row count cannot: the same number of rows with one value
// changed or truncated, and an extra row on either side.
func TestRequireSameContent(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T, oldRows, newRows []string) *sql.Tx {
		t.Helper()
		dir := t.TempDir()
		db, err := sql.Open("sqlite", filepath.Join(dir, "new.db"))
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = db.Close() })
		o := filepath.Join(dir, "old.db")
		if _, err = db.Exec(`ATTACH DATABASE ? AS old`, o); err != nil {
			t.Fatal(err)
		}
		for _, s := range []string{"main", "old"} {
			if _, err = db.Exec(`CREATE TABLE ` + s + `.t (id INTEGER PRIMARY KEY, body TEXT)`); err != nil {
				t.Fatal(err)
			}
		}
		for _, r := range oldRows {
			if _, err = db.Exec(`INSERT INTO old.t (id, body) VALUES ` + r); err != nil { //nolint:gosec // fixed test rows
				t.Fatal(err)
			}
		}
		for _, r := range newRows {
			if _, err = db.Exec(`INSERT INTO main.t (id, body) VALUES ` + r); err != nil { //nolint:gosec // fixed test rows
				t.Fatal(err)
			}
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = tx.Rollback() })
		return tx
	}
	cols := `"id", "body"`
	if err := requireSameContent(ctx, setup(t, []string{"(1,'a')", "(2,NULL)"}, []string{"(1,'a')", "(2,NULL)"}), "t", cols); err != nil {
		t.Fatalf("identical content refused: %v", err)
	}
	for name, tc := range map[string]struct {
		oldRows, newRows []string
		want             string
	}{
		"value changed, same count": {[]string{"(1,'hello world')"}, []string{"(1,'hello wor')"}, "missing or changed"},
		"null became empty":         {[]string{"(1,NULL)"}, []string{"(1,'')"}, "missing or changed"},
		"row missing in new":        {[]string{"(1,'a')", "(2,'b')"}, []string{"(1,'a')"}, "missing or changed"},
		"extra row in new":          {[]string{"(1,'a')"}, []string{"(1,'a')", "(2,'b')"}, "not in the old one"},
	} {
		t.Run(name, func(t *testing.T) {
			err := requireSameContent(ctx, setup(t, tc.oldRows, tc.newRows), "t", cols)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// A writer that touches the old file while migrate-data is checking must stop the run before the
// rename, and leave nothing at the target.
func TestMigrateLegacyDetectsAWriterOnTheOldFile(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "main.db")
	legacyDBInternal(t, old)
	afterChecksHook = func() {
		db, err := sql.Open("sqlite", old)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = db.Close() }()
		if _, err = db.Exec(`INSERT INTO wiki_bundles (slug, title) VALUES ('late','Late')`); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(func() { afterChecksHook = nil })
	to := NewPath(old)
	_, err := MigrateLegacy(context.Background(), old, to)
	if err == nil || !strings.Contains(err.Error(), "changed during the migration") {
		t.Fatalf("err = %v, want one saying the old database changed", err)
	}
	for _, f := range []string{to, to + ".migrating"} {
		if _, serr := os.Stat(f); !errors.Is(serr, os.ErrNotExist) {
			t.Fatalf("%s exists after a failed run: %v", f, serr)
		}
	}
}

// legacyDBInternal builds the old database from the fixture migrations (the external tests'
// legacyDB lives in package storage_test).
func legacyDBInternal(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.Exec(`CREATE TABLE schema_version (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')))`); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob("testdata/legacy/*.sql")
	if err != nil || len(files) != 9 {
		t.Fatalf("legacy migrations: %v %v", files, err)
	}
	for _, f := range files {
		body, rerr := os.ReadFile(f) //nolint:gosec // test fixture path
		if rerr != nil {
			t.Fatal(rerr)
		}
		if _, err = db.Exec(string(body)); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if _, err = db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, strings.TrimSuffix(filepath.Base(f), ".sql")); err != nil {
			t.Fatal(err)
		}
	}
}
