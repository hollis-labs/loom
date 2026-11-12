package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/pflag"

	"github.com/hollis-labs/substrate/modules/cli"
)

// Any command that only reads or describes must leave the tree it runs in byte for byte as it
// found it: after the new binary is installed, the first innocent `loom status` or `loom path` in
// a checkout must not create an empty loom.db and a signing key beside an unmigrated main.db.
// Only serve, mcp, identity setup, identity key create and the commands that write data may create
// anything. These tests run the real binary.

// snapshot describes every file and directory under roots: name, and for files size, mode and a
// content hash. Modification times are left out on purpose: a created or changed file shows in its
// name, size or hash, and opening a database read-only may touch a directory's time without
// changing anything.
func snapshot(t *testing.T, roots ...string) []string {
	t.Helper()
	var out []string
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return fs.SkipDir // a missing root is an empty tree
			}
			if d.IsDir() {
				out = append(out, "dir  "+p)
				return nil
			}
			b, _ := os.ReadFile(p) //nolint:gosec // a path inside this test's temp tree
			sum := sha256.Sum256(b)
			info, _ := d.Info()
			out = append(out, "file "+p+" "+info.Mode().String()+" "+hex.EncodeToString(sum[:8]))
			return nil
		})
	}
	sort.Strings(out)
	return out
}

// oldLayoutDB writes a pre-Substrate database (schema_version plus one bundle) at path.
func oldLayoutDB(t *testing.T, path string) {
	t.Helper()
	fixtures, err := filepath.Abs("../../internal/storage/testdata/legacy")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = old.Close() }()
	if _, err = old.Exec(`CREATE TABLE schema_version (version TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(fixtures, "*.sql"))
	for _, f := range files {
		body, _ := os.ReadFile(f) //nolint:gosec // test fixture path
		if _, err = old.Exec(string(body)); err != nil {
			t.Fatal(err)
		}
		if _, err = old.Exec(`INSERT INTO schema_version (version) VALUES (?)`, strings.TrimSuffix(filepath.Base(f), ".sql")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = old.Exec(`INSERT INTO wiki_bundles (id, slug, title) VALUES (5,'kept','Kept')`); err != nil {
		t.Fatal(err)
	}
}

func TestReadOnlyCommandsCreateNothing(t *testing.T) {
	bin := loomBinary(t)
	run := func(home, dir string, stdin string, args ...string) (string, error) {
		c := exec.Command(bin, args...) //nolint:gosec // the binary this test just built
		c.Dir = dir
		c.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "XDG_DATA_HOME=" + home + "/data",
			"XDG_STATE_HOME=" + home + "/state", "XDG_CACHE_HOME=" + home + "/cache", "XDG_CONFIG_HOME=" + home + "/config"}
		c.Stdin = strings.NewReader(stdin)
		out, err := c.CombinedOutput()
		return string(out), err
	}
	// Three places the first innocent command can run: an empty home; a home holding only an old
	// main.db (the served instance's layout); a project checkout holding only an old .loom.
	// `--no-project` selects the home's data directory (the served instance's layout); without it a
	// checkout's own .loom is selected, and the home's old file is not looked at at all.
	scenarios := map[string]func(t *testing.T) (home, dir string){
		"empty home": func(t *testing.T) (string, string) { return t.TempDir(), t.TempDir() },
		"home with only an old main.db": func(t *testing.T) (string, string) {
			home := t.TempDir()
			oldLayoutDB(t, filepath.Join(home, "data", "loom", "workspaces", "default", "main.db"))
			return home, t.TempDir()
		},
		"checkout with only an old .loom": func(t *testing.T) (string, string) {
			dir := t.TempDir()
			oldLayoutDB(t, filepath.Join(dir, ".loom", "data", "workspaces", "default", "main.db"))
			return t.TempDir(), dir
		},
	}
	commands := [][]string{
		{"--help"}, {"help"}, {"-h"}, {"version"}, {"path"}, {"status"}, {"config"}, {"health"},
		{"--no-project", "path"}, {"--no-project", "status"}, {"--no-project", "version"},
	}
	for name, setup := range scenarios {
		for _, args := range commands {
			t.Run(name+"/"+strings.Join(args, " "), func(t *testing.T) {
				home, dir := setup(t)
				before := snapshot(t, home, dir)
				out, err := run(home, dir, "", args...)
				after := snapshot(t, home, dir)
				if strings.Join(before, "\n") != strings.Join(after, "\n") {
					t.Fatalf("%v changed the tree\nbefore:\n%s\nafter:\n%s\noutput: %s", args, strings.Join(before, "\n"), strings.Join(after, "\n"), out)
				}
				// The commands that describe must work; status on an old layout may refuse, naming the way out.
				switch args[len(args)-1] {
				case "status":
					noProject := slices.Contains(args, "--no-project")
					seesOld := (name == "home with only an old main.db" && noProject) || (name == "checkout with only an old .loom" && !noProject)
					if seesOld && (err == nil || !strings.Contains(out, "migrate-data")) {
						t.Fatalf("status on an old layout = %v %q, want a refusal naming migrate-data", err, out)
					}
					if !seesOld && (err != nil || !strings.Contains(out, "no_database")) {
						t.Fatalf("status with no database = %v %q, want no_database and exit 0", err, out)
					}
				case "health":
					// health with no database module reports only what it can; it must not fail.
				default:
					if err != nil {
						t.Fatalf("%v failed: %v\n%s", args, err, out)
					}
				}
			})
		}
	}

	// The positive control: a command that is allowed to create does, and the snapshot sees it, so
	// the checks above are known to be able to detect a write.
	home, dir := t.TempDir(), t.TempDir()
	before := snapshot(t, home, dir)
	if out, err := run(home, dir, "correct horse battery staple\n", "--no-project", "identity", "setup", "--username", "owner"); err != nil {
		t.Fatalf("control: identity setup: %v\n%s", err, out)
	}
	if strings.Join(before, "\n") == strings.Join(snapshot(t, home, dir), "\n") {
		t.Fatal("control: identity setup created nothing, so the checks above prove nothing")
	}
}

// status on a database that already exists and is migrated reports its counts, and needs no
// module that creates anything else: no signing key appears.
func TestStatusReadsAnExistingDatabaseWithoutCreatingAKey(t *testing.T) {
	bin := loomBinary(t)
	home := t.TempDir()
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "XDG_DATA_HOME=" + home + "/data",
		"XDG_STATE_HOME=" + home + "/state", "XDG_CACHE_HOME=" + home + "/cache", "XDG_CONFIG_HOME=" + home + "/config"}
	do := func(args ...string) string {
		c := exec.Command(bin, args...) //nolint:gosec // the binary this test just built
		c.Dir = home
		c.Env = env
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	do("--no-project", "templates", "put", "-name", "t", "-generator", "wiki_page", "-body", "x") // a command that writes data creates the database
	key := filepath.Join(home, "data", "loom", "identity-signing.key")
	if err := os.Remove(key); err != nil {
		t.Fatalf("expected the data command to have created a signing key to remove: %v", err)
	}
	_ = os.Remove(key + ".jwks.json")
	if out := do("--no-project", "status"); !strings.Contains(out, `"status":"ok"`) {
		t.Fatalf("status on an existing database = %s", out)
	}
	if _, err := os.Stat(key); err == nil {
		t.Fatal("status created a signing key")
	}
}

// plan reads every argument that does not start with "-" as a command word, which is right only
// while no root-level flag takes a value. Fails when one is added.
func TestRootHasNoValueFlags(t *testing.T) {
	layout, err := resolveLayout([]string{"-no-project"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := startModulesFor(t.Context(), layout, false, pickModules(readOnlyModules), false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = run.Stop(t.Context()) }()
	root := cli.Root(run.App)
	root.AddCommand((&loom{app: run.App}).serveCmd(t.Context()))
	check := func(fs *pflag.FlagSet) {
		fs.VisitAll(func(f *pflag.Flag) {
			if f.Value.Type() != "bool" {
				t.Errorf("root flag --%s takes a value (%s); plan() would read its value as the command word, teach it first", f.Name, f.Value.Type())
			}
		})
	}
	check(root.Flags())
	check(root.PersistentFlags())
}
