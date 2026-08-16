package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hollis-labs/loom/internal/mcp"
	"github.com/hollis-labs/loom/internal/service"
	"github.com/hollis-labs/loom/internal/storage"
)

var wikiResultPointer = regexp.MustCompile(`wiki_result://([0-9a-fA-F-]+)`)

// TestMCPPageSearchCacheDeepDive exercises CW-20260816-0014's per-caller
// cache end to end through the public MCP tool surface: loom_page_search
// returns a byte-large result, gets a wiki_result://<id> pointer appended to
// its hint, and loom_fetch_result resolves that pointer back to the full,
// untruncated result for the same caller_id - while a different caller_id
// is refused.
func TestMCPPageSearchCacheDeepDive(t *testing.T) {
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
	srv := mcp.NewServer(repo, service.NewCompiler(repo))

	// Each page's body is big enough that three of them, marshaled together,
	// comfortably exceed the 2 KiB default soft-truncation threshold.
	bigBody := strings.Repeat("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ", 40)
	for i := 0; i < 3; i++ {
		input, err := json.Marshal(map[string]any{
			"title": fmt.Sprintf("Cache Deep Dive Page %d", i),
			"body":  bigBody,
		})
		if err != nil {
			t.Fatalf("marshal input %d: %v", i, err)
		}
		if _, err := srv.CallTool(ctx, "loom_compile_request", map[string]any{
			"bundle": "nanite", "generator": "wiki_page", "input": string(input),
		}); err != nil {
			t.Fatalf("compile page %d: %v", i, err)
		}
	}

	raw, err := srv.CallTool(ctx, "loom_page_search", map[string]any{
		"bundle": "nanite", "q": "Cache Deep Dive", "caller_id": "tester-1",
	})
	if err != nil {
		t.Fatalf("page search: %v", err)
	}

	var env struct {
		Count int    `json:"count"`
		Total int    `json:"total"`
		Hint  string `json:"hint"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal search envelope: %v (raw=%s)", err, raw)
	}
	if env.Total != 3 {
		t.Fatalf("total = %d, want 3", env.Total)
	}
	if !strings.Contains(env.Hint, "wiki_result://") {
		t.Fatalf("expected a wiki_result:// pointer in hint for a byte-large result, got: %q", env.Hint)
	}

	m := wikiResultPointer.FindStringSubmatch(env.Hint)
	if m == nil {
		t.Fatalf("could not extract cache id from hint: %q", env.Hint)
	}
	id := m[1]

	// Deep-dive by id for the caller that produced the search.
	fetchRaw, err := srv.CallTool(ctx, "loom_fetch_result", map[string]any{"id": id, "caller_id": "tester-1"})
	if err != nil {
		t.Fatalf("fetch_result: %v", err)
	}
	var fetched struct {
		TotalSize int    `json:"total_size"`
		Data      string `json:"data"`
	}
	if err := json.Unmarshal([]byte(fetchRaw), &fetched); err != nil {
		t.Fatalf("unmarshal fetch_result: %v (raw=%s)", err, fetchRaw)
	}
	if fetched.TotalSize == 0 {
		t.Fatal("expected non-zero total_size")
	}

	var fullPages []struct {
		Slug string `json:"slug"`
		Body string `json:"body"`
	}
	if err := json.Unmarshal([]byte(fetched.Data), &fullPages); err != nil {
		t.Fatalf("unmarshal cached full page list: %v (data=%s)", err, fetched.Data)
	}
	if len(fullPages) != 3 {
		t.Fatalf("cached full result has %d pages, want 3", len(fullPages))
	}
	for _, p := range fullPages {
		if !strings.Contains(p.Body, "Lorem ipsum") {
			t.Fatalf("cached page %q missing expected full body content", p.Slug)
		}
	}

	// loom_search_result should locate one specific item inside the cached
	// blob by pattern (e.g. a slug) without paging through it by offset.
	searchRaw, err := srv.CallTool(ctx, "loom_search_result", map[string]any{
		"id": id, "caller_id": "tester-1", "pattern": "cache-deep-dive-page-1",
	})
	if err != nil {
		t.Fatalf("search_result: %v", err)
	}
	var searched struct {
		Matches []struct {
			Match string `json:"match"`
		} `json:"matches"`
	}
	if err := json.Unmarshal([]byte(searchRaw), &searched); err != nil {
		t.Fatalf("unmarshal search_result: %v (raw=%s)", err, searchRaw)
	}
	if len(searched.Matches) == 0 {
		t.Fatal("expected loom_search_result to find the target page's slug in the cached result")
	}

	// A different caller_id must not be able to read this caller's cache.
	if _, err := srv.CallTool(ctx, "loom_fetch_result", map[string]any{"id": id, "caller_id": "someone-else"}); err == nil {
		t.Fatal("expected error fetching another caller's cached result")
	}

	// loom_fetch_result requires an id.
	if _, err := srv.CallTool(ctx, "loom_fetch_result", map[string]any{"caller_id": "tester-1"}); err == nil {
		t.Fatal("expected error when id is omitted")
	}
}
