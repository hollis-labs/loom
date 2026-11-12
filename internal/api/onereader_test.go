package api_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hollis-labs/loom/internal/api"
	"github.com/hollis-labs/loom/internal/service"
	"github.com/hollis-labs/loom/internal/storage/storagetest"
)

// The store has one write connection and a read pool; here the pool is one connection. Every
// route runs against it, concurrently with writes, under a deadline: a handler that holds a read
// cursor or a transaction while it asks for another connection hangs and fails the test.
func TestEveryRouteWithOneReadConnection(t *testing.T) {
	repo, closeStore, err := storagetest.OpenRepository(t.Context(), filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()
	h := api.New(repo, service.NewCompiler(repo))
	mux := h.Mux()

	do := func(method, path, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		rec := httptest.NewRecorder()
		done := make(chan struct{})
		go func() { mux.ServeHTTP(rec, req); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("%s %s did not return: blocked on the database", method, path)
		}
		return rec.Code
	}
	if c := do(http.MethodPost, "/api/compile-jobs", `{"bundle":"nanite","generator":"wiki_page","input":"# One\n\nSee [[Two]]\n"}`); c != http.StatusOK {
		t.Fatalf("seed compile = %d", c)
	}
	subst := strings.NewReplacer("{slug}", "nanite", "{bundle}", "nanite", "{id}", "1", "{hash}", "x", "{name}", "wiki_page.default")
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, rt := range api.Routes() {
				path := subst.Replace(rt.Pattern)
				if rt.Method == http.MethodGet {
					do(http.MethodGet, path, "")
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 5 {
			do(http.MethodPost, "/api/compile-jobs", `{"bundle":"nanite","generator":"wiki_page","input":"# Again\n\nBody\n"}`)
			do(http.MethodPost, "/api/ingest/text", `{"bundle":"nanite","text":"hello","source":"t"}`)
		}
	}()
	wg.Wait()
	// A page route with a real page: list, then each per-page read.
	for _, p := range []string{"/api/pages/nanite/one", "/api/pages/nanite/one/links", "/api/pages/nanite/one/verifications", "/api/pages/nanite/one/conformance", "/api/bundles/nanite/conformance"} {
		if c := do(http.MethodGet, p, ""); c != http.StatusOK {
			t.Errorf("GET %s = %d", p, c)
		}
	}
}
