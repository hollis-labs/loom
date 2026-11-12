package storage

import (
	"context"
	"database/sql"
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
