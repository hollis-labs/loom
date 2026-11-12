package mcp_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/hollis-labs/loom/internal/mcp"
	"github.com/hollis-labs/loom/internal/mcp/mcptest"
	"github.com/hollis-labs/loom/internal/service"
	"github.com/hollis-labs/loom/internal/storage/storagetest"
)

// Every tool runs against a store whose read pool is one connection, under a deadline. A tool
// that holds a cursor or transaction while it waits for a second connection hangs and fails.
// Tool errors (missing arguments) are fine; only a call that does not return is a failure.
func TestEveryToolWithOneReadConnection(t *testing.T) {
	repo, closeStore, err := storagetest.OpenRepository(t.Context(), filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()
	srv := mcptest.New(repo, service.NewCompiler(repo))
	if _, err = srv.CallTool(t.Context(), "loom_compile_request", map[string]any{"bundle": "nanite", "generator": "wiki_page", "input": "# One\n\nBody"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	args := map[string]any{
		"bundle": "nanite", "slug": "one", "page": "one", "q": "One", "id": 1, "job_id": 1, "hash": "x", "name": "wiki_page.default",
		"generator": "wiki_page", "input": "# Two\n\nBody", "title": "Two", "body": "x", "text": "hello", "limit": 5,
	}
	for _, tool := range mcp.ToolNames() {
		done := make(chan struct{})
		go func() {
			defer close(done)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			_, _ = srv.CallTool(ctx, tool, args)
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("tool %s did not return: blocked on the database", tool)
		}
	}
}
