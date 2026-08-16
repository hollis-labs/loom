package mcp

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hollis-labs/go-mcp/budget"
	"github.com/hollis-labs/loom/internal/storage"
)

// setupCacheTest opens a fresh migrated temp-file SQLite DB (so the real
// wiki_result_cache schema from migration 009 is exercised, not a hand-
// rolled test table) and returns a ResultCache plus the raw *sql.DB for
// tests that need to reach around the cache API (e.g. to force-expire a
// row without sleeping).
func setupCacheTest(t *testing.T, cfg ResultCacheConfig) (*ResultCache, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := storage.Open(ctx, filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := storage.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewResultCache(db, cfg), db
}

func TestResultCache_StoreAndFetch(t *testing.T) {
	cache, _ := setupCacheTest(t, ResultCacheConfig{})

	body := strings.Repeat("x", 500)
	id, expiresAt, err := cache.Store("caller-1", "call-1", "test_tool", body)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}
	if !expiresAt.After(time.Now().UTC()) {
		t.Fatalf("expiresAt %v should be in the future", expiresAt)
	}

	slice, total, err := cache.Fetch("caller-1", id, 0, 0)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if total != len(body) {
		t.Fatalf("total = %d, want %d", total, len(body))
	}
	if slice != body {
		t.Fatal("fetched body does not match stored body")
	}

	// Offset/length paging.
	slice2, _, err := cache.Fetch("caller-1", id, 10, 20)
	if err != nil {
		t.Fatalf("fetch with offset: %v", err)
	}
	if slice2 != body[10:30] {
		t.Fatalf("slice2 = %q, want body[10:30]", slice2)
	}

	// Unknown id.
	if _, _, err := cache.Fetch("caller-1", "does-not-exist", 0, 0); err == nil {
		t.Fatal("expected error fetching unknown id")
	}
}

func TestResultCache_PerCallerScoping(t *testing.T) {
	cache, _ := setupCacheTest(t, ResultCacheConfig{})

	id, _, err := cache.Store("caller-a", "call-1", "test_tool", "secret result for caller-a")
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	if _, _, err := cache.Fetch("caller-b", id, 0, 0); err == nil {
		t.Fatal("expected error: caller-b must not read caller-a's cached result")
	}
	if _, err := cache.Search("caller-b", id, "secret", 0); err == nil {
		t.Fatal("expected error: caller-b must not search caller-a's cached result")
	}
	if _, _, err := cache.Fetch("caller-a", id, 0, 0); err != nil {
		t.Fatalf("owning caller fetch should succeed: %v", err)
	}
}

func TestResultCache_TTLExpiry(t *testing.T) {
	cache, db := setupCacheTest(t, ResultCacheConfig{})

	id, _, err := cache.Store("caller-1", "call-1", "test_tool", "body that will expire")
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	// Force the row into the past instead of sleeping out a real TTL.
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE wiki_result_cache SET expires_at = ? WHERE id = ?`, past, id); err != nil {
		t.Fatalf("force-expire: %v", err)
	}

	if _, _, err := cache.Fetch("caller-1", id, 0, 0); err == nil {
		t.Fatal("expected expired error from Fetch")
	} else if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("Fetch error = %v, want mention of expiry", err)
	}
	if _, err := cache.Search("caller-1", id, "body", 0); err == nil {
		t.Fatal("expected expired error from Search")
	} else if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("Search error = %v, want mention of expiry", err)
	}
}

func TestResultCache_HardCap(t *testing.T) {
	cache, _ := setupCacheTest(t, ResultCacheConfig{HardCapBytes: 100})

	body := strings.Repeat("y", 200) // over the 100-byte hard cap
	id, _, err := cache.Store("caller-1", "call-1", "test_tool", body)
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	if _, _, err := cache.Fetch("caller-1", id, 0, 0); err == nil {
		t.Fatal("expected hard-cap error from Fetch")
	} else if !strings.Contains(err.Error(), "hard cap") {
		t.Fatalf("Fetch error = %v, want mention of hard cap", err)
	}
}

func TestResultCache_Purge(t *testing.T) {
	cache, db := setupCacheTest(t, ResultCacheConfig{})

	idExpired, _, err := cache.Store("caller-1", "call-1", "test_tool", "expired body")
	if err != nil {
		t.Fatalf("store expired: %v", err)
	}
	idValid, _, err := cache.Store("caller-1", "call-2", "test_tool", "valid body")
	if err != nil {
		t.Fatalf("store valid: %v", err)
	}

	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE wiki_result_cache SET expires_at = ? WHERE id = ?`, past, idExpired); err != nil {
		t.Fatalf("force-expire: %v", err)
	}

	n, err := cache.Purge()
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 1 {
		t.Fatalf("purged = %d, want 1", n)
	}
	if _, _, err := cache.Fetch("caller-1", idValid, 0, 0); err != nil {
		t.Fatalf("valid entry should survive purge: %v", err)
	}
	if _, _, err := cache.Fetch("caller-1", idExpired, 0, 0); err == nil {
		t.Fatal("expired entry should be gone after purge")
	}
}

func TestResultCache_Search(t *testing.T) {
	cache, _ := setupCacheTest(t, ResultCacheConfig{})

	body := "line one\nfoo match here\nline three\nanother foo line\nline five"
	id, _, err := cache.Store("caller-1", "call-1", "test_tool", body)
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	matches, err := cache.Search("caller-1", id, "foo", 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("matches = %d, want 2 (%+v)", len(matches), matches)
	}
}

func TestCacheListResult_CachesFullSetWhenOverThreshold(t *testing.T) {
	cache, _ := setupCacheTest(t, ResultCacheConfig{SoftTruncBytes: 40})

	items := []string{"aaaaaaaaaa", "bbbbbbbbbb", "cccccccccc", "dddddddddd"}
	out := cacheListResult(cache, "caller-1", "test_list_tool", items, budget.Config{Limit: 10}, "")
	if !strings.Contains(out, "wiki_result://") {
		t.Fatalf("expected wiki_result:// pointer in output, got: %s", out)
	}
}

func TestCacheListResult_NoCacheWhenUnderThreshold(t *testing.T) {
	cache, _ := setupCacheTest(t, ResultCacheConfig{}) // default 2KiB threshold

	items := []string{"a", "b"}
	out := cacheListResult(cache, "caller-1", "test_list_tool", items, budget.Config{Limit: 10}, "")
	if strings.Contains(out, "wiki_result://") {
		t.Fatalf("did not expect a cache pointer for a tiny result, got: %s", out)
	}
}

func TestCacheListResult_NilCacheIsNoOp(t *testing.T) {
	items := []string{"a", "b", "c"}
	out := cacheListResult(nil, "caller-1", "test_list_tool", items, budget.Config{Limit: 10}, "")
	if strings.Contains(out, "wiki_result://") {
		t.Fatalf("nil cache must never produce a cache pointer, got: %s", out)
	}
}
