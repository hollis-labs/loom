package exporter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/hollis-labs/loom/internal/storage"
)

// openExportDir anchors every subsequent operation to an operator-owned root.
// os.Root enforces containment during resolution, including concurrent symlink
// changes; a lexical prefix check alone cannot provide that guarantee.
func openExportDir(base, dir, bundle string) (*os.Root, string, error) {
	refuse := func(err error) (*os.Root, string, error) {
		return nil, "", fmt.Errorf("%w: export confined to paths.export_dir: %v", storage.ErrInvalid, err)
	}
	if strings.TrimSpace(base) == "" {
		return refuse(fmt.Errorf("no export root configured"))
	}
	base, err := filepath.Abs(base)
	if err != nil {
		return refuse(err)
	}
	if dir == "" {
		dir = bundle
	}
	rel := dir
	if filepath.IsAbs(dir) {
		rel, err = filepath.Rel(base, dir)
		if err != nil {
			return refuse(err)
		}
	}
	rel = filepath.Clean(rel)
	if rel != "." && !filepath.IsLocal(rel) {
		return refuse(fmt.Errorf("directory %q escapes export root", dir))
	}
	// Only this operator-supplied base may be created outside an existing root.
	if err := os.MkdirAll(base, 0o755); err != nil {
		return refuse(err)
	}
	canonical, err := filepath.EvalSymlinks(base)
	if err != nil {
		return refuse(err)
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return refuse(err)
	}
	defer root.Close()
	if err := root.MkdirAll(rel, 0o755); err != nil {
		return refuse(err)
	}
	child, err := root.OpenRoot(rel)
	if err != nil {
		return refuse(err)
	}
	return child, filepath.Join(canonical, rel), nil
}

// Replace entries atomically rather than truncating an existing file: an
// existing hardlink must not let an export modify a file outside this root.
func writeArtifact(root *os.Root, name string, data []byte) error {
	if filepath.Base(name) != name || name == "." || name == ".." {
		return fmt.Errorf("%w: invalid export artifact name %q", storage.ErrInvalid, name)
	}
	if info, err := root.Lstat(name); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: export artifact %q is a symlink", storage.ErrInvalid, name)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	tmp := ".loom-export-" + uuid.NewString()
	file, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	_, err = file.Write(data)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return root.Rename(tmp, name)
}
