package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite" // the stand-in for a second writer holds a transaction with it
)

// These tests answer the cutover question with the real binary: after cutover the served
// instance (loom serve, no -db) and the gateway's stdio child (loom mcp -db <the same file>)
// open one database. Nothing is stubbed: two processes, one file, real modules.

var (
	buildOnce sync.Once
	builtBin  string
	buildErr  error
)

// removeBuilt deletes the binary these tests built; TestMain calls it after the run.
func removeBuilt() {
	if builtBin != "" {
		os.RemoveAll(filepath.Dir(builtBin)) //nolint:errcheck,gosec // best-effort temp cleanup
	}
}

func loomBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "loom-bin-")
		if err != nil {
			buildErr = err
			return
		}
		builtBin = filepath.Join(dir, "loom")
		out, err := exec.Command("go", "build", "-o", builtBin, ".").CombinedOutput() //nolint:gosec // fixed arguments
		if err != nil {
			buildErr = fmt.Errorf("go build: %w\n%s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return builtBin
}

// syncBuf is a log buffer a test can read while the process is still writing to it.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// host is one operating-system user's worth of state: HOME and every XDG directory in a temp
// directory, so neither process can reach the real installation.
type host struct {
	t    *testing.T
	home string
	addr string
	cfg  string
	noDB bool // the child is launched without -db, from the same config as the service
}

func newHost(t *testing.T) *host {
	t.Helper()
	home := t.TempDir()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	h := &host{t: t, home: home, addr: addr}
	h.cfg = filepath.Join(home, "override.yaml")
	h.writeConfig("")
	return h
}

func (h *host) writeConfig(extra string) {
	h.t.Helper()
	if err := os.WriteFile(h.cfg, []byte("httphost:\n  addr: \""+h.addr+"\"\n"+extra), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

func (h *host) env() []string {
	return append(os.Environ(), "HOME="+h.home, "XDG_DATA_HOME="+filepath.Join(h.home, "data"),
		"XDG_STATE_HOME="+filepath.Join(h.home, "state"), "XDG_CACHE_HOME="+filepath.Join(h.home, "cache"),
		"XDG_CONFIG_HOME="+filepath.Join(h.home, "config"), "SUBSTRATE_CONFIG="+h.cfg)
}

// dataDir is where the served instance (no -db) keeps its identity key; workspaceDir is where
// the default workspace keeps its database, and so where an explicit -db <that file> child
// keeps its.
func (h *host) dataDir() string      { return filepath.Join(h.home, "data", "loom") }
func (h *host) workspaceDir() string { return filepath.Join(h.dataDir(), "workspaces", "default") }
func (h *host) db() string           { return filepath.Join(h.workspaceDir(), "loom.db") }

func (h *host) command(args ...string) *exec.Cmd {
	cmd := exec.Command(loomBinary(h.t), args...) //nolint:gosec // the binary this test just built
	cmd.Env = h.env()
	cmd.Dir = h.home
	return cmd
}

// service starts `loom serve --no-project` and returns once it answers.
func (h *host) service() *exec.Cmd {
	h.t.Helper()
	var log syncBuf
	cmd := h.command("serve", "--no-project")
	cmd.Stdout, cmd.Stderr = &log, &log
	if err := cmd.Start(); err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			_ = cmd.Process.Kill()
		}
	})
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if res, err := http.Get("http://" + h.addr + "/api/setup"); err == nil { //nolint:noctx // a poll against a loopback test server
			_ = res.Body.Close()
			return cmd
		}
		time.Sleep(100 * time.Millisecond)
	}
	h.t.Fatalf("the service never answered:\n%s", log.String())
	return nil
}

// child runs `loom mcp --no-project [-db <db>]` with the token in a file (none: local trust), sends one tools/call and
// returns the reply line and how long the call took. extraArgs go before -db.
func (h *host) child(token, tool string, args map[string]any, extraArgs ...string) (string, time.Duration, error) {
	h.t.Helper()
	if token == "" { // no token: the child runs under local filesystem trust
		h.writeConfig("")
	} else {
		tokenFile := filepath.Join(h.home, "stdio.token")
		if err := os.WriteFile(tokenFile, []byte(token), 0o600); err != nil {
			h.t.Fatal(err)
		}
		h.writeConfig("mcp:\n  stdio_token_file: " + tokenFile + "\n")
	}
	argv := append([]string{"mcp", "--no-project"}, extraArgs...)
	if !h.noDB {
		argv = append(argv, "-db", h.db())
	}
	cmd := h.command(argv...)
	in, err := cmd.StdinPipe()
	if err != nil {
		h.t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		h.t.Fatal(err)
	}
	var stderr syncBuf
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		h.t.Fatal(err)
	}
	defer func() {
		_ = in.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	send := func(v map[string]any) {
		b, _ := json.Marshal(v)
		_, _ = in.Write(append(b, '\n'))
	}
	replies := make(chan string, 8)
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			replies <- sc.Text()
		}
		close(replies)
	}()
	send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}})
	send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	start := time.Now()
	send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}})
	timeout := time.After(30 * time.Second)
	for {
		select {
		case line, ok := <-replies:
			if !ok {
				return "", time.Since(start), fmt.Errorf("child exited without answering: %s", strings.TrimSpace(stderr.String()))
			}
			var m struct {
				ID int `json:"id"`
			}
			if json.Unmarshal([]byte(line), &m) == nil && m.ID == 2 {
				return line, time.Since(start), nil
			}
		case <-timeout:
			return "", time.Since(start), fmt.Errorf("child never answered: %s", strings.TrimSpace(stderr.String()))
		}
	}
}

// The fix for the split: launch the child like the service, with the same SUBSTRATE_CONFIG and
// no -db. It then resolves the same data directory, so the same signing key and the same
// database, and writes no second key.
func TestChildWithoutDBSharesTheServiceKeyAndFile(t *testing.T) {
	h := newHost(t)
	key := h.owner()
	h.service()
	serviceKey := filepath.Join(h.dataDir(), "identity-signing.key")
	before, err := os.ReadFile(serviceKey) //nolint:gosec // a path inside this test's temp home
	if err != nil {
		t.Fatal(err)
	}
	h.noDB = true
	line, _, err := h.child("", "loom_template_put", map[string]any{"name": "from-child", "generator": "wiki_page", "body": "x"})
	if err != nil || isToolError(line) {
		t.Fatalf("child without -db: %v %s", err, line)
	}
	if _, statErr := os.Stat(filepath.Join(h.workspaceDir(), "identity-signing.key")); statErr == nil {
		t.Fatal("the child wrote a second signing key beside the database")
	}
	after, _ := os.ReadFile(serviceKey) //nolint:gosec // a path inside this test's temp home
	if !bytes.Equal(before, after) {
		t.Fatal("the service's signing key changed")
	}
	get, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+h.addr+"/api/templates/from-child", nil)
	get.Header.Set("Authorization", "Bearer "+key)
	res, err := http.DefaultClient.Do(get)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("the service does not see the write of a child launched without -db: %d", res.StatusCode)
	}
}

// owner is the offline bootstrap, run before any server: `loom identity setup` (password on
// stdin) and `loom identity key create --stdout`. It returns the key. It writes the database, the
// signing key and the owner exactly where the service will look for them.
func (h *host) owner() string {
	h.t.Helper()
	setup := h.command("--no-project", "identity", "setup", "--username", "owner")
	setup.Stdin = strings.NewReader(ownerPassword + "\n")
	if out, err := setup.CombinedOutput(); err != nil {
		h.t.Fatalf("identity setup: %v\n%s", err, out)
	}
	create := h.command("--no-project", "identity", "key", "create", "--name", "stdio", "--scope", "loom.mcp", "--scope", "loom.read", "--scope", "loom.write", "--stdout")
	var stderr syncBuf
	create.Stderr = &stderr
	out, err := create.Output()
	if err != nil {
		h.t.Fatalf("identity key create: %v\n%s", err, stderr.String())
	}
	key := strings.TrimSpace(string(out))
	if !strings.HasPrefix(key, "key_") {
		h.t.Fatalf("identity key create printed %q", out)
	}
	return key
}

func isToolError(line string) bool {
	return strings.Contains(line, `"isError":true`) || strings.Contains(line, `"error":`)
}

// (a), (b) and the second-issuer guard: the default workspace's database directory is not the data
// directory, so a child launched with -db <that file> would look for its signing key beside the
// database, not where the service keeps it. The framework refuses to create a signing key when the
// database already holds users and no key file exists where the config points, so the wrongly
// launched child stops instead of quietly becoming a second issuer.
func TestChildWithDBRefusesToBecomeASecondIssuer(t *testing.T) {
	h := newHost(t)
	h.owner()
	if h.dataDir() == h.workspaceDir() {
		t.Fatal("the data directory and the default workspace directory are the same; the premise is gone")
	}
	if _, err := os.Stat(filepath.Join(h.dataDir(), "identity-signing.key")); err != nil {
		t.Fatalf("the bootstrap did not put the signing key in the data directory: %v", err)
	}
	_, _, err := h.child("", "loom_status", map[string]any{}) // launched with -db
	if err == nil {
		t.Fatal("a child launched with -db against a database that has users started")
	}
	if !strings.Contains(err.Error(), "signing key") {
		t.Fatalf("the child stopped, but not for the signing key: %v", err)
	}
	t.Logf("refused: %v", err)
	if _, statErr := os.Stat(filepath.Join(h.workspaceDir(), "identity-signing.key")); statErr == nil {
		t.Fatal("the refused child still wrote a signing key beside the database")
	}
}

// (b): a key made offline authenticates on the service's /mcp and, given as the stdio token, on a
// child's stdio; a wrong token is an error, never a fallback to local trust. The child is launched
// the way the cutover launches it: the service's SUBSTRATE_CONFIG, no -db.
func TestKeyWorksOnBothTransportsAndABadTokenDoesNotFallBack(t *testing.T) {
	h := newHost(t)
	key := h.owner()
	h.service()
	h.noDB = true
	line, _, err := h.child(key, "loom_status", map[string]any{})
	if err != nil || isToolError(line) {
		t.Fatalf("stdio child with a key: %v %s", err, line)
	}
	if _, _, err = h.child("key_not-a-real-key", "loom_status", map[string]any{}); err == nil {
		t.Fatal("a child with a wrong token started; it must not fall back to local trust")
	}
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "loom_status", "arguments": map[string]any{}}})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://"+h.addr+"/mcp", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || isToolError(string(raw)) {
		t.Fatalf("service /mcp with the key = %d %s", res.StatusCode, raw)
	}
}

// Local trust: the child needs no key file. It runs as the verified local operator, so the result
// cache (owned by the verified principal) works: a list result too big to return inline leaves a
// pointer, and loom_fetch_result on that pointer returns it.
func TestLocalTrustChildNeedsNoKeyAndKeepsTheResultCache(t *testing.T) {
	h := newHost(t)
	h.owner()
	h.noDB = true
	for i := range 40 {
		line, _, err := h.child("", "loom_bundle_put", map[string]any{"slug": fmt.Sprintf("bundle-%02d", i), "title": strings.Repeat("title ", 8)})
		if err != nil || isToolError(line) {
			t.Fatalf("local-trust write %d: %v %s", i, err, line)
		}
	}
	line, _, err := h.child("", "loom_bundle_list", map[string]any{})
	if err != nil || isToolError(line) {
		t.Fatalf("local-trust list: %v %s", err, line)
	}
	m := regexp.MustCompile(`wiki_result://([A-Za-z0-9-]+)`).FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("a large list left no result pointer (no verified principal, or the result was not cached): %s", line)
	}
	line, _, err = h.child("", "loom_fetch_result", map[string]any{"id": m[1]})
	if err != nil || isToolError(line) {
		t.Fatalf("fetching the cached result as the same local principal: %v %s", err, line)
	}
}

// A local-trust child holds exactly loom.mcp, loom.read and loom.write; nothing else is granted by
// the "*" default. (The scope list is set in code at the call site, not from config.)
func TestLocalTrustScopesAreListed(t *testing.T) {
	src, err := os.ReadFile("serve.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `LocalScopes: []string{"loom.mcp", "loom.read", "loom.write"}`) {
		t.Fatal("serve.go does not pass an explicit LocalScopes list to mcp.ServeStdio")
	}
}

// raceStart starts the service and a token-less child at the same moment and returns each one's
// output once the child has stopped. The child stops after its start-up with the token error,
// so seeing that error proves it got through the migrations and the identity start-up.
func raceStart(t *testing.T, h *host) (svc *exec.Cmd, svcLog, childLog string) {
	t.Helper()
	var sl, cl syncBuf
	svc = h.command("serve", "--no-project")
	svc.Stdout, svc.Stderr = &sl, &sl
	kidArgs := []string{"mcp", "--no-project"}
	if !h.noDB {
		kidArgs = append(kidArgs, "-db", h.db())
	}
	kid := h.command(kidArgs...)
	kid.Stderr = &cl
	kid.Stdin = strings.NewReader("")
	h.writeConfig("") // local trust; the child ends at once (stdin is closed) once it has started
	var wg sync.WaitGroup
	start := make(chan struct{})
	t.Cleanup(func() { // whatever path the test leaves by, no process outlives it
		for _, c := range []*exec.Cmd{svc, kid} {
			if c.Process != nil {
				_ = c.Process.Kill()
				_ = c.Wait()
			}
		}
	})
	for _, c := range []*exec.Cmd{svc, kid} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := c.Start(); err != nil {
				t.Error(err)
			}
		}()
	}
	close(start)
	wg.Wait()
	done := make(chan struct{})
	go func() { _ = kid.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		_ = kid.Process.Kill()
		t.Fatal("child did not exit")
	}
	time.Sleep(2 * time.Second) // the service either listens by now or has exited
	return svc, sl.String(), cl.String()
}

func stop(c *exec.Cmd) {
	if c.Process == nil {
		return
	}
	_ = c.Process.Signal(os.Interrupt)
	done := make(chan struct{})
	go func() { _ = c.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		_ = c.Process.Kill()
		<-done
	}
}

func (h *host) answers() bool {
	res, err := http.Get("http://" + h.addr + "/api/setup") //nolint:noctx // loopback poll
	if err != nil {
		return false
	}
	_ = res.Body.Close()
	return true
}

func (h *host) integrity() {
	h.t.Helper()
	db, err := sql.Open("sqlite", "file:"+h.db()+"?mode=ro")
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var check string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&check); err != nil || check != "ok" {
		h.t.Fatalf("integrity_check = %q, %v", check, err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM identity_users`).Scan(&n); err != nil {
		h.t.Fatalf("identity tables: %v", err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM wiki_bundles`).Scan(&n); err != nil {
		h.t.Fatalf("application tables: %v", err)
	}
}

// raceRounds starts the service and a child (the cutover's launch: same config, no -db) at the
// same moment, rounds times, each round on a fresh host. With premigrated the file already exists,
// migrated, with its signing key (what migrate-data plus the bootstrap leave); otherwise it is
// empty. Simultaneous starts are NOT safe either way (see the tests below): the assertion is that
// the file is never damaged and a lone start afterwards comes up, and the outcome is logged.
func raceRounds(t *testing.T, rounds int, premigrated bool) (lost int) {
	t.Helper()
	for round := range rounds {
		h := newHost(t)
		h.noDB = true
		if premigrated {
			h.owner()
		} else if err := os.MkdirAll(h.workspaceDir(), 0o750); err != nil {
			t.Fatal(err)
		}
		svc, svcLog, childLog := raceStart(t, h)
		survived := h.answers()
		stop(svc)
		childOK := strings.Contains(childLog, "local filesystem trust")
		if !survived || !childOK {
			lost++
		}
		t.Logf("round %d: service up=%v, child got through start-up=%v\n  service: %s  child: %s", round, survived, childOK, strings.TrimSpace(svcLog), strings.TrimSpace(childLog))
		lone := h.command("serve", "--no-project")
		if err := lone.Start(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(20 * time.Second)
		for !h.answers() {
			if time.Now().After(deadline) {
				stop(lone)
				t.Fatalf("round %d: a lone start after the race never came up", round)
			}
			time.Sleep(100 * time.Millisecond)
		}
		stop(lone)
		h.integrity()
	}
	t.Logf("%d of %d simultaneous starts lost the race in at least one process", lost, rounds)
	return lost
}

// (c), the first start: both processes meet an empty file and both run the migrations and create
// the signing key. NOT safe: one of them can lose at open (database is locked) or at the
// migration, or the two collide on the key's files. The cutover never starts the two together on
// an empty file (migrate-data creates the schema, the bootstrap the key and the owner first).
func TestConcurrentFirstStartLeavesAUsableFile(t *testing.T) { raceRounds(t, 5, false) }

// (c), a start on a migrated file with its key: simultaneous starts can still collide, because
// both processes rewrite the key's JWKS file (write a temp file, rename it), so the order is the
// rule here too: the service first, the child once the service answers.
func TestSimultaneousStartOnAMigratedFileLeavesAUsableFile(t *testing.T) { raceRounds(t, 3, true) }

// (c), the order that is the rule: the child starts, and restarts, while the service is running.
// This is what the gateway does when the child exits and is started again.
func TestChildStartsAndRestartsWhileTheServiceRuns(t *testing.T) {
	h := newHost(t)
	h.noDB = true
	h.owner()
	h.service()
	for i := range 3 {
		line, _, err := h.child("", "loom_status", map[string]any{})
		if err != nil || isToolError(line) {
			t.Fatalf("child start %d while the service runs: %v %s", i, err, line)
		}
		if !h.answers() {
			t.Fatalf("the service stopped answering after child start %d", i)
		}
	}
	h.integrity()
}

// holdWrite takes the database's write lock from a third connection (standing in for the
// service in the middle of a write transaction) and releases it after d.
func holdWrite(t *testing.T, path string, d time.Duration) (locked <-chan struct{}) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(0)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	ch := make(chan struct{})
	close(ch)
	go func() {
		time.Sleep(d)
		_, _ = conn.ExecContext(ctx, `ROLLBACK`)
		_ = conn.Close()
		_ = db.Close()
	}()
	return ch
}

// (d): a write through the child while another connection holds the write lock waits for it (up to
// the 5s busy timeout) and succeeds if the lock is released in time; past that it fails busy.
func TestChildWriteWaitsForTheWriteLock(t *testing.T) {
	h := newHost(t)
	h.owner()
	h.service()
	h.noDB = true

	put := func(name string) (string, time.Duration, error) {
		return h.child("", "loom_template_put", map[string]any{"name": name, "generator": "wiki_page", "body": "x"})
	}
	if line, _, err := put("warm"); err != nil || isToolError(line) {
		t.Fatalf("an unlocked write failed: %v %s", err, line)
	}

	<-holdWrite(t, h.db(), 1500*time.Millisecond)
	line, took, err := put("waited")
	if err != nil || isToolError(line) {
		t.Fatalf("a write during a 1.5s lock failed instead of waiting: %v %s", err, line)
	}
	if took < time.Second {
		t.Fatalf("the write took %s: it did not wait for the lock", took)
	}
	t.Logf("write during a 1.5s hold: waited %s and succeeded", took.Round(10*time.Millisecond))

	// A hold longer than the 5s busy timeout is NOT waited out: a single-statement write goes through
	// the one write connection directly (not Store.WriteTx, which is the only thing that retries
	// on busy), so it waits the busy timeout once and then fails with SQLITE_BUSY.
	<-holdWrite(t, h.db(), 8*time.Second)
	line, took, err = put("too-late")
	if err != nil {
		t.Fatal(err)
	}
	if !isToolError(line) || !strings.Contains(line, "SQLITE_BUSY") {
		t.Fatalf("a write during an 8s lock was expected to fail busy after the 5s timeout, got %s after %s", line, took)
	}
	if took < 4*time.Second || took > 7*time.Second {
		t.Fatalf("the busy failure came after %s, not about the 5s busy timeout", took)
	}
	t.Logf("write during an 8s hold: failed busy after %s", took.Round(100*time.Millisecond))
	time.Sleep(4 * time.Second) // let the hold end before the host is torn down
}
