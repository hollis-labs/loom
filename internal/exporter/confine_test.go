package exporter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExportDirectoryHandleSurvivesPathReplacement(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	root, _, err := openExportDir(base, "child", "nanite")
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.Rename(filepath.Join(base, "child"), filepath.Join(base, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "child")); err != nil {
		t.Fatal(err)
	}
	if err := writeArtifact(root, "index.md", []byte("export")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outside, "index.md")); !os.IsNotExist(err) {
		t.Fatalf("write escaped pinned directory: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(base, "moved", "index.md")); err != nil || string(data) != "export" {
		t.Fatalf("pinned write: %q %v", data, err)
	}
	for _, name := range []string{"../escape.md", "subdir/file.md", "/absolute.md"} {
		if err := writeArtifact(root, name, []byte("bad")); err == nil {
			t.Fatalf("accepted artifact %q", name)
		}
	}
}
