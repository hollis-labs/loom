// Package storagetest opens a migrated Loom database the way Substrate's database module
// does (libs core/store, then the embedded goose migrations), for tests that need their own
// database, a reopen, or several handles on one file. It is imported by tests only.
package storagetest

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hollis-labs/libs/core/store"
	"github.com/hollis-labs/libs/core/store/migrate"

	"github.com/hollis-labs/loom/internal/storage"
)

// Open opens and migrates the SQLite database at path, creating it when missing. The
// returned handle is the store's writer; closing it closes the store's writer only, so tests
// that care about the read pool should use OpenStore.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	st, err := OpenStore(ctx, path)
	if err != nil {
		return nil, err
	}
	return st.DB(), nil
}

// OpenStore is Open returning the whole store.
func OpenStore(ctx context.Context, path string) (*store.Store, error) {
	if path == "" {
		return nil, fmt.Errorf("db path is required")
	}
	cfg := store.Config{Driver: store.DriverSQLite, SQLite: store.SQLiteConfig{Path: path, CreateParentDir: true}}
	st, err := store.Open(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	fsys, root := storage.Migrations()
	if err := migrate.Ensure(ctx, cfg, st, migrate.Source{FS: fsys, Root: root}); err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return st, nil
}

// OpenRepository opens a migrated database the way the running app does: the repository writes
// through the store's single write connection and reads through its read pool, which is cut to
// one connection here so a read that waits for a second one deadlocks the test instead of
// passing on a pool of four. The store is closed with the test.
func OpenRepository(ctx context.Context, path string) (*storage.Repository, func(), error) {
	cfg := store.Config{Driver: store.DriverSQLite, SQLite: store.SQLiteConfig{Path: path, CreateParentDir: true, ReadConns: 1}}
	st, err := store.Open(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}
	fsys, root := storage.Migrations()
	if err = migrate.Ensure(ctx, cfg, st, migrate.Source{FS: fsys, Root: root}); err != nil {
		_ = st.Close()
		return nil, nil, fmt.Errorf("migrate: %w", err)
	}
	return storage.NewRepositoryRW(st.DB(), st.Reader()), func() { _ = st.Close() }, nil
}
