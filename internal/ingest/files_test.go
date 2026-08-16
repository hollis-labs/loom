package ingest_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hollis-labs/loom/internal/ingest"
)

func TestFileSourceReadsTextFilesWithExcludesAndRedaction(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "alpha.md"), []byte("# Alpha\n\nsecret=abc123\n"), 0o644); err != nil {
		t.Fatalf("write alpha: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "skip"), 0o755); err != nil {
		t.Fatalf("mkdir skip: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skip", "beta.md"), []byte("# Beta\n\nignore"), 0o644); err != nil {
		t.Fatalf("write beta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.bin"), []byte("ignore"), 0o644); err != nil {
		t.Fatalf("write bin: %v", err)
	}

	items, err := (ingest.FileSource{
		Paths:   []string{dir},
		Source:  "fixture",
		Exclude: []string{"skip"},
		Redact:  []string{`secret=\w+`},
	}).Items(context.Background())
	if err != nil {
		t.Fatalf("Items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %+v, want one", items)
	}
	if items[0].Title != "Alpha" || items[0].Source != "fixture" {
		t.Fatalf("item = %+v", items[0])
	}
	if !strings.Contains(items[0].Body, "[REDACTED]") || strings.Contains(items[0].Body, "abc123") {
		t.Fatalf("body was not redacted: %q", items[0].Body)
	}
	if items[0].ContentHash == "" {
		t.Fatalf("missing content hash")
	}
}

func TestFileSourceInvalidInputsUseSentinel(t *testing.T) {
	if _, err := (ingest.FileSource{}).Items(context.Background()); !errors.Is(err, ingest.ErrInvalid) {
		t.Fatalf("empty paths err = %v, want ErrInvalid", err)
	}
	if _, err := (ingest.FileSource{Paths: []string{filepath.Join(t.TempDir(), "missing.md")}}).Items(context.Background()); !errors.Is(err, ingest.ErrInvalid) {
		t.Fatalf("missing path err = %v, want ErrInvalid", err)
	}
	if _, err := (ingest.FileSource{Paths: []string{t.TempDir()}, Redact: []string{"["}}).Items(context.Background()); !errors.Is(err, ingest.ErrInvalid) {
		t.Fatalf("bad redaction err = %v, want ErrInvalid", err)
	}
}
