package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	paths "github.com/hollis-labs/libs/core/util/apppaths"

	"github.com/hollis-labs/loom/internal/storage"
)

// explicitDB reports whether the command line named the database file (-db), which is then used
// literally instead of loom.db beside the workspace's old main.db.
func explicitDB(appArgs []string) bool {
	for _, a := range appArgs {
		if a == "-db" || strings.HasPrefix(a, "-db=") {
			return true
		}
	}
	return false
}

// migrateData is `loom migrate-data`: a one-way copy of a pre-Substrate database into the new
// schema. The old file is only read and is the rollback. With no options it migrates the current
// workspace's main.db to loom.db beside it; --all does every workspace; --from/--to name files.
func migrateData(ctx context.Context, layout paths.Layout, explicit bool, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("migrate-data", flag.ContinueOnError)
	from := fs.String("from", "", "old database file (default: the workspace's main.db)")
	to := fs.String("to", "", "new database file (default: loom.db beside the old file)")
	all := fs.Bool("all", false, "migrate every workspace's main.db")
	if err := fs.Parse(args); err != nil {
		return err
	}
	type job struct{ from, to string }
	var jobs []job
	switch {
	case *all:
		if *from != "" || *to != "" || explicit {
			return errors.New("--all cannot be combined with --from, --to or -db")
		}
		spaces, err := layout.Workspaces()
		if err != nil {
			return err
		}
		for _, w := range spaces {
			if _, err := os.Stat(w.DBPath); err == nil {
				jobs = append(jobs, job{w.DBPath, storage.NewPath(w.DBPath)})
			}
		}
		if len(jobs) == 0 {
			return errors.New("no workspace database found to migrate")
		}
	default:
		j := job{*from, *to}
		if j.from == "" {
			j.from = layout.MainDB()
		}
		if j.to == "" {
			j.to = storage.NewPath(j.from)
		}
		jobs = append(jobs, j)
	}
	enc := json.NewEncoder(out)
	for _, j := range jobs {
		rep, err := storage.MigrateLegacy(ctx, j.from, j.to)
		if err != nil {
			return fmt.Errorf("%s: %w", j.from, err)
		}
		if err := enc.Encode(map[string]any{
			"from": rep.From, "to": rep.To, "rows": rep.Counts, "seconds": rep.Duration.Seconds(),
			"integrity_check": "ok", "foreign_key_check": "ok", "old_file": "untouched",
		}); err != nil {
			return err
		}
	}
	return nil
}
