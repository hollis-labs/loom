package directivex_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/hollis-labs/loom/internal/directivex"
	"github.com/hollis-labs/loom/internal/storage"
)

func TestParseAndLedgerMarksNewOnlyOnce(t *testing.T) {
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := storage.NewRepository(db)
	text := "::context_start Loom\nnotes\n::note Capture this\n::context_end"

	first, err := directivex.ParseAndLedger(ctx, repo, text, "test")
	if err != nil {
		t.Fatalf("first parse: %v", err)
	}
	second, err := directivex.ParseAndLedger(ctx, repo, text, "test")
	if err != nil {
		t.Fatalf("second parse: %v", err)
	}
	if len(first.Directives) != 1 || !first.Directives[0].New {
		t.Fatalf("first directives = %+v, want one new directive", first.Directives)
	}
	if len(second.Directives) != 1 || second.Directives[0].New {
		t.Fatalf("second directives = %+v, want duplicate directive", second.Directives)
	}
}
