package storage_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hollis-labs/loom/internal/storage"
	"github.com/hollis-labs/loom/internal/storage/storagetest"
)

// legacyDB builds a database the way the pre-Substrate migrator did (the nine numbered files in
// testdata/legacy and its schema_version ledger), filled with synthetic rows only.
func legacyDB(t *testing.T, path string, skipVersion string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.Exec(`PRAGMA foreign_keys=ON; CREATE TABLE schema_version (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')))`); err != nil {
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
		v := strings.TrimSuffix(filepath.Base(f), ".sql")
		if v != skipVersion {
			if _, err = db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	// The old Go seed() ran after the migrations; reproduce it, then add synthetic data.
	for _, q := range []string{
		`INSERT INTO wiki_bundles (slug, title, description) VALUES ('nanite','Nanite','edited seed')`,
		`INSERT INTO templates (name, generator, body) VALUES ('wiki_page.default','wiki_page','# {{title}}

{{body}}')`,
		`INSERT INTO wiki_bundles (id, slug, title, scope) VALUES (7,'team','Team','meta')`,
		`INSERT INTO wiki_pages (id, bundle_id, slug, title, body, tags, sources) VALUES (11,7,'alpha','Alpha','# Alpha','["x"]','["s"]'),(12,7,'beta','Beta','# Beta [a](alpha)','[]','[]'),(40,1,'gamma','Gamma','body','[]','[]')`,
		`INSERT INTO wiki_links (id, from_page_id, to_page_id, target, kind, label, position, relation) VALUES (3,12,11,'alpha','wiki','a',0,'references'),(4,12,NULL,'https://example.test','external','',1,'references')`,
		`INSERT INTO wiki_verifications (id, page_id, kind, status, message, "by") VALUES (5,11,'heading','passed','','loom-structural-checker')`,
		`INSERT INTO compile_jobs (id, bundle_id, generator, status, input) VALUES (21,7,'wiki_page','completed','in')`,
		`INSERT INTO compile_events (id, job_id, status, message) VALUES (31,21,'queued',''),(32,21,'completed','')`,
		`INSERT INTO directive_ledger (hash, command, prompt, line) VALUES ('h1','note','p',3)`,
		`INSERT INTO templates (id, name, generator, body) VALUES (9,'custom','wiki_page','# {{title}}')`,
		`INSERT INTO ingest_ledger (hash, source, source_key, content_hash, job_id) VALUES ('i1','file','k','c',21)`,
		`INSERT INTO wiki_result_cache (id, caller_id, tool_name, tool_call_id, expires_at, byte_size) VALUES ('c1','someone','loom_page_search','t','2999-01-01T00:00:00Z',1)`,
	} {
		if _, err = db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

func count(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMigrateLegacyCopiesEverythingWithKeys(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "main.db")
	legacyDB(t, old, "")
	before, err := os.ReadFile(old) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatal(err)
	}
	to := storage.NewPath(old)
	rep, err := storage.MigrateLegacy(context.Background(), old, to)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Counts["wiki_pages"] != 3 || rep.Counts["wiki_links"] != 2 || rep.Counts["compile_events"] != 2 || rep.Counts["wiki_bundles"] != 2 {
		t.Fatalf("counts = %v", rep.Counts)
	}
	after, err := os.ReadFile(old) //nolint:gosec // test fixture path
	if err != nil || string(before) != string(after) {
		t.Fatalf("the old database changed (err %v)", err)
	}
	// Open the result the way the database module does, and read it back through the repository.
	db, err := storagetest.Open(context.Background(), to)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := storage.NewRepository(db)
	b, err := repo.GetBundle(context.Background(), "nanite")
	if err != nil || b.ID != 1 || b.Description != "edited seed" {
		t.Fatalf("seed bundle = %+v %v (the old row must win over the new seed)", b, err)
	}
	p, err := repo.GetPageBySlug(context.Background(), "team", "beta")
	if err != nil || p.ID != 12 {
		t.Fatalf("page = %+v %v", p, err)
	}
	var to2 sql.NullInt64
	if err = db.QueryRow(`SELECT to_page_id FROM wiki_links WHERE id=3`).Scan(&to2); err != nil || to2.Int64 != 11 {
		t.Fatalf("link target = %v %v", to2, err)
	}
	var by string
	if err = db.QueryRow(`SELECT "by" FROM wiki_verifications WHERE id=5`).Scan(&by); err != nil || by != "loom-structural-checker" {
		t.Fatalf("by = %q %v", by, err)
	}
	if n := count(t, db, "wiki_result_cache"); n != 0 {
		t.Fatalf("result cache copied: %d rows", n)
	}
	if n := count(t, db, "templates"); n != 2 {
		t.Fatalf("templates = %d", n)
	}
	// New ids continue after the highest migrated one.
	res, err := db.Exec(`INSERT INTO wiki_bundles (slug, title) VALUES ('later','Later')`)
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := res.LastInsertId(); id != 8 {
		t.Fatalf("next bundle id = %d, want 8", id)
	}
	if _, err = os.Stat(to + ".migrating"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary file left behind: %v", err)
	}
}

func TestMigrateLegacyRefusesASecondRun(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "main.db")
	legacyDB(t, old, "")
	to := storage.NewPath(old)
	if _, err := storage.MigrateLegacy(context.Background(), old, to); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.MigrateLegacy(context.Background(), old, to); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second run err = %v", err)
	}
}

func TestMigrateLegacyLeavesNothingOnFailure(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "main.db")
	legacyDB(t, old, "009_wiki_result_cache")
	to := storage.NewPath(old)
	if _, err := storage.MigrateLegacy(context.Background(), old, to); err == nil || !strings.Contains(err.Error(), "009_wiki_result_cache") {
		t.Fatalf("err = %v, want one naming the missing migration", err)
	}
	if _, err := os.Stat(to); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new database exists after a failed run: %v", err)
	}
	if _, err := storage.MigrateLegacy(context.Background(), filepath.Join(dir, "none.db"), to); err == nil {
		t.Fatal("missing source accepted")
	}
}

func TestRequireMigrated(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "main.db")
	cur := storage.NewPath(old)
	if err := storage.RequireMigrated(old, cur); err != nil {
		t.Fatalf("no old database: %v", err)
	}
	legacyDB(t, old, "")
	if err := storage.RequireMigrated(old, cur); !errors.Is(err, storage.ErrLegacyLayout) || !strings.Contains(err.Error(), "migrate-data") {
		t.Fatalf("unmigrated old database: err = %v", err)
	}
	if _, err := storage.MigrateLegacy(context.Background(), old, cur); err != nil {
		t.Fatal(err)
	}
	if err := storage.RequireMigrated(old, cur); err != nil {
		t.Fatalf("after migration: %v", err)
	}
}

// An old file with anything the copy would not carry stops the run, naming it, and leaves no new file.
func TestMigrateLegacyRefusesWhatItWouldDrop(t *testing.T) {
	for name, tc := range map[string]struct{ ddl, want string }{
		"unlisted table":      {`CREATE TABLE notes (id INTEGER PRIMARY KEY, body TEXT)`, `table "notes"`},
		"trigger":             {`CREATE TRIGGER tr AFTER INSERT ON templates BEGIN SELECT 1; END`, `trigger "tr"`},
		"view":                {`CREATE VIEW v AS SELECT slug FROM wiki_bundles`, `view "v"`},
		"virtual table":       {`CREATE VIRTUAL TABLE ft USING fts5(body)`, `"ft"`},
		"autoincrement":       {`CREATE TABLE seq (id INTEGER PRIMARY KEY AUTOINCREMENT)`, `AUTOINCREMENT`},
		"extra old column":    {`ALTER TABLE wiki_pages ADD COLUMN secret TEXT`, `wiki_pages.secret`},
		"changed column type": {`ALTER TABLE templates RENAME COLUMN body TO body_old; ALTER TABLE templates ADD COLUMN body INTEGER; ALTER TABLE templates DROP COLUMN body_old`, `templates`},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			old := filepath.Join(dir, "main.db")
			legacyDB(t, old, "")
			db, err := sql.Open("sqlite", old)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(tc.ddl); err != nil {
				t.Skipf("this SQLite cannot build the case: %v", err)
			}
			_ = db.Close()
			to := storage.NewPath(old)
			if _, err = storage.MigrateLegacy(context.Background(), old, to); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one containing %q", err, tc.want)
			}
			if _, err = os.Stat(to); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("new database exists after a refused run: %v", err)
			}
		})
	}
}

// The old schema (the nine numbered migrations) has no AUTOINCREMENT, trigger, view or virtual
// table, so the copy has no sequence or object to carry. The refusals above cover the day it does.
func TestLegacySchemaHasNoSequencesOrTriggers(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "main.db")
	legacyDB(t, old, "")
	db, err := sql.Open("sqlite", old)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type IN ('trigger','view') OR name='sqlite_sequence' OR upper(sql) LIKE '%AUTOINCREMENT%' OR upper(sql) LIKE '%VIRTUAL TABLE%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("the old schema has %d sequence/trigger/view/virtual objects", n)
	}
}

// snapshot is the directory listing (names and sizes) plus the sha256 of one file.
func snapshot(t *testing.T, dir, file string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, e := range entries {
		info, ierr := e.Info()
		if ierr != nil {
			t.Fatal(ierr)
		}
		b.WriteString(e.Name() + ":" + fmt.Sprint(info.Size()) + "\n")
	}
	data, err := os.ReadFile(file) //nolint:gosec // test path
	if err != nil {
		t.Fatal(err)
	}
	return b.String() + fmt.Sprintf("%x", sha256.Sum256(data))
}

// The guards read the old file to judge it. They must not change it or add a -wal or -shm
// beside it, including when the file is in WAL mode.
func TestLegacyGuardsLeaveTheOldFileAlone(t *testing.T) {
	for _, journal := range []string{"DELETE", "WAL"} {
		t.Run(journal, func(t *testing.T) {
			dir := t.TempDir()
			old := filepath.Join(dir, "main.db")
			legacyDB(t, old, "")
			db, err := sql.Open("sqlite", old)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`PRAGMA journal_mode=` + journal); err != nil {
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
			before := snapshot(t, dir, old)
			if err = storage.RefuseLegacy(old); !errors.Is(err, storage.ErrLegacyLayout) {
				t.Fatalf("RefuseLegacy = %v, want ErrLegacyLayout", err)
			}
			if err = storage.RequireMigrated(old, storage.NewPath(old)); err == nil {
				t.Fatal("RequireMigrated accepted an unmigrated directory")
			}
			if after := snapshot(t, dir, old); after != before {
				t.Fatalf("the old file or its directory changed:\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}

// migrate-data leaves the old file and its neighbors (-wal, -shm) as found, WAL mode included.
func TestMigrateLegacyLeavesTheOldFilesAlone(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "main.db")
	legacyDB(t, old, "")
	db, err := sql.Open("sqlite", old)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	names := func() string {
		entries, _ := os.ReadDir(dir)
		var b strings.Builder
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "main.db") {
				info, _ := e.Info()
				b.WriteString(e.Name() + ":" + fmt.Sprint(info.Size()) + "\n")
			}
		}
		return b.String()
	}
	before := names()
	if _, err = storage.MigrateLegacy(context.Background(), old, storage.NewPath(old)); err != nil {
		t.Fatal(err)
	}
	if after := names(); after != before {
		t.Fatalf("the old file's neighbors changed:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// A WAL that still holds content (a writer that has not checkpointed) is read, not ignored: the
// guard must still see the old layout in it.
func TestRefuseLegacyReadsALiveWAL(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "main.db")
	db, err := sql.Open("sqlite", old)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	for _, q := range []string{`PRAGMA journal_mode=WAL`, `PRAGMA wal_autocheckpoint=0`, `CREATE TABLE schema_version (version TEXT)`} {
		if _, err = db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if fi, serr := os.Stat(old + "-wal"); serr != nil || fi.Size() == 0 {
		t.Skip("this SQLite did not leave content in the WAL")
	}
	if err = storage.RefuseLegacy(old); !errors.Is(err, storage.ErrLegacyLayout) {
		t.Fatalf("RefuseLegacy on a live WAL = %v, want ErrLegacyLayout", err)
	}
}
