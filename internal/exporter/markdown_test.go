package exporter_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hollis-labs/loom/internal/domain"
	"github.com/hollis-labs/loom/internal/exporter"
	"github.com/hollis-labs/loom/internal/storage"
)

func TestExportBundleWritesIndexLogAndPages(t *testing.T) {
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
	b, _ := repo.GetBundle(ctx, "nanite")
	if _, err := repo.UpsertPage(ctx, domain.Page{BundleID: b.ID, Slug: "overview", Title: "Overview", Summary: "Intro", Body: "# Overview\n\nSee [Runtime](runtime)."}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	dir := t.TempDir()
	exp, err := exporter.ExportBundle(ctx, repo, "nanite", dir)
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	if len(exp.Files) != 3 {
		t.Fatalf("files = %+v, want index/log/page", exp.Files)
	}
	index, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if !strings.Contains(string(index), "[Overview](overview.md)") {
		t.Fatalf("index = %s", string(index))
	}
	page, err := os.ReadFile(filepath.Join(dir, "overview.md"))
	if err != nil {
		t.Fatalf("read page: %v", err)
	}
	if !strings.Contains(string(page), "## Links") || !strings.Contains(string(page), "`wiki` [Runtime](runtime)") {
		t.Fatalf("page missing links section = %s", string(page))
	}
	if !strings.Contains(string(page), "## Verifications") || !strings.Contains(string(page), "| heading | passed |") {
		t.Fatalf("page missing verifications section = %s", string(page))
	}
	log, err := os.ReadFile(filepath.Join(dir, "log.md"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(log), "- links: `1`") || !strings.Contains(string(log), "- verification_warnings: `1`") {
		t.Fatalf("log missing audit counts = %s", string(log))
	}
}

func TestExportBundleEscapesMarkdownControlCharacters(t *testing.T) {
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
	b, err := repo.GetBundle(ctx, "nanite")
	if err != nil {
		t.Fatalf("GetBundle: %v", err)
	}
	_, err = repo.UpsertPage(ctx, domain.Page{
		BundleID: b.ID,
		Slug:     "escape-check",
		Title:    "Escape: Check",
		Summary:  "pipe | newline\nsummary",
		Body:     "# Escape: Check\n\nSee [Runtime Guide](runtime docs) and [[Other Page]].",
	})
	if err != nil {
		t.Fatalf("UpsertPage: %v", err)
	}
	dir := t.TempDir()
	if _, err := exporter.ExportBundle(ctx, repo, "nanite", dir); err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	page, err := os.ReadFile(filepath.Join(dir, "escape-check.md"))
	if err != nil {
		t.Fatalf("read page: %v", err)
	}
	text := string(page)
	if !strings.Contains(text, `title: "Escape: Check"`) || !strings.Contains(text, `summary: "pipe | newline\nsummary"`) {
		t.Fatalf("front matter not quoted safely:\n%s", text)
	}
	if !strings.Contains(text, `[Runtime Guide](runtime%20docs)`) {
		t.Fatalf("link target not escaped:\n%s", text)
	}
	if !strings.Contains(text, `page summary is present`) || strings.Contains(text, "newline\nsummary |") {
		t.Fatalf("verification table malformed:\n%s", text)
	}
}
