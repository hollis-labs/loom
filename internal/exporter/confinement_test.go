package exporter_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hollis-labs/loom/internal/api"
	"github.com/hollis-labs/loom/internal/exporter"
	"github.com/hollis-labs/loom/internal/mcp"
	"github.com/hollis-labs/loom/internal/service"
	"github.com/hollis-labs/loom/internal/storage"
)

// Every exposed surface uses the same confinement boundary and no live state.
func TestExportConfinementAcrossSurfaces(t *testing.T) {
	for _, surface := range []string{"exporter", "mcp", "http"} {
		for _, name := range []string{"default", "relative", "absolute-inside", "outside", "sibling-prefix", "traversal", "symlink-dir", "symlink-file", "hardlink-file", "missing-root"} {
			t.Run(surface+"/"+name, func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				base := filepath.Join(home, "exports")
				outside := filepath.Join(home, "catalog")
				for _, dir := range []string{base, outside} {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				victim := filepath.Join(outside, "index.md")
				if err := os.WriteFile(victim, []byte("protected"), 0o644); err != nil {
					t.Fatal(err)
				}
				db, err := storage.Open(context.Background(), filepath.Join(home, "loom.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				if err := storage.Migrate(context.Background(), db); err != nil {
					t.Fatal(err)
				}
				repo := storage.NewRepository(db)
				dir := ""
				allowed := false
				switch name {
				case "default":
					allowed = true
				case "relative":
					dir = "child/nested"
					allowed = true
				case "absolute-inside":
					dir = filepath.Join(base, "child")
					allowed = true
				case "outside":
					dir = outside
				case "sibling-prefix":
					dir = base + "-other"
				case "traversal":
					dir = "../catalog"
				case "symlink-dir":
					dir = "escape/new"
					if err := os.Symlink(outside, filepath.Join(base, "escape")); err != nil {
						t.Fatal(err)
					}
				case "symlink-file":
					dir = "."
					if err := os.Symlink(victim, filepath.Join(base, "index.md")); err != nil {
						t.Fatal(err)
					}
				case "hardlink-file":
					dir = "."
					allowed = true
					if err := os.Link(victim, filepath.Join(base, "index.md")); err != nil {
						t.Fatal(err)
					}
				case "missing-root":
					base = ""
				}
				switch surface {
				case "exporter":
					_, err = exporter.ExportBundle(context.Background(), repo, "nanite", dir, base)
				case "mcp":
					srv := mcp.NewServerWithOptions(repo, service.NewCompiler(repo), mcp.Options{ExportRoot: base})
					_, err = srv.CallTool(context.Background(), "loom_export_bundle", map[string]any{"bundle": "nanite", "dir": dir})
				case "http":
					mux := http.NewServeMux()
					api.New(repo, service.NewCompiler(repo)).WithExportRoot(base).Register(mux)
					body, _ := json.Marshal(map[string]string{"dir": dir})
					rec := httptest.NewRecorder()
					mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/bundles/nanite/export", strings.NewReader(string(body))))
					want := http.StatusBadRequest
					if allowed {
						want = http.StatusOK
					}
					if rec.Code != want {
						t.Fatalf("status %d want %d: %s", rec.Code, want, rec.Body.String())
					}
					if rec.Code != http.StatusOK {
						err = &exportError{rec.Body.String()}
					}
				}
				if (err == nil) != allowed {
					t.Fatalf("allowed=%v error=%v", allowed, err)
				}
				if !allowed && !strings.Contains(err.Error(), "export") {
					t.Fatalf("missing export refusal: %v", err)
				}
				data, err := os.ReadFile(victim)
				if err != nil || string(data) != "protected" {
					t.Fatalf("outside victim changed: %q %v", data, err)
				}
				if _, err := os.Stat(filepath.Join(outside, "new")); !os.IsNotExist(err) {
					t.Fatalf("created outside directory: %v", err)
				}
				if allowed {
					target := dir
					if target == "" {
						target = "nanite"
					}
					if !filepath.IsAbs(target) {
						target = filepath.Join(base, target)
					}
					if _, err := os.Stat(filepath.Join(target, "index.md")); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

type exportError struct{ message string }

func (e *exportError) Error() string { return e.message }
