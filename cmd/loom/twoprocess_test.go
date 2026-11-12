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

// child runs `loom mcp --no-project -db <db>` with the token in a file, sends one tools/call and
// returns the reply line and how long the call took. extraArgs go before -db.
func (h *host) child(token, tool string, args map[string]any, extraArgs ...string) (string, time.Duration, error) {
	h.t.Helper()
	tokenFile := filepath.Join(h.home, "stdio.token")
	if err := os.WriteFile(tokenFile, []byte(token), 0o600); err != nil {
		h.t.Fatal(err)
	}
	h.writeConfig("mcp:\n  stdio_token_file: " + tokenFile + "\n")
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
	h.service()
	_, key := h.owner()
	serviceKey := filepath.Join(h.dataDir(), "identity-signing.key")
	before, err := os.ReadFile(serviceKey) //nolint:gosec // a path inside this test's temp home
	if err != nil {
		t.Fatal(err)
	}
	h.noDB = true
	line, _, err := h.child(key, "loom_template_put", map[string]any{"name": "from-child", "generator": "wiki_page", "body": "x"})
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

// owner signs the first owner up on the running service and returns a session and a write key.
func (h *host) owner() (*session, string) {
	h.t.Helper()
	s := newSession(h.t, "http://"+h.addr)
	s.want(http.StatusOK, "POST", "/api/setup", map[string]any{"display_name": "Owner", "username": "owner", "password": "correct horse battery staple"})
	out := s.want(http.StatusCreated, "POST", "/api/identity/keys", map[string]any{"name": "stdio", "scopes": []string{"loom.mcp", "loom.read", "loom.write"}})
	key, _ := out["key"].(string)
	if key == "" {
		h.t.Fatalf("no key in %v", out)
	}
	return s, key
}

func isToolError(line string) bool {
	return strings.Contains(line, `"isError":true`) || strings.Contains(line, `"error":`)
}

// (a) and (b): the default workspace's database directory is not the data directory, so the
// service and an explicit-`-db` child keep two different signing keys; a key created through the
// service is a database row and authenticates on both transports regardless.
func TestServiceAndDBChildKeepTwoKeysButShareCredentials(t *testing.T) {
	h := newHost(t)
	h.service()
	_, key := h.owner()

	if h.dataDir() == h.workspaceDir() {
		t.Fatal("the data directory and the default workspace directory are the same; the premise of the finding is gone")
	}
	serviceKey := filepath.Join(h.dataDir(), "identity-signing.key")
	if _, err := os.Stat(serviceKey); err != nil {
		t.Fatalf("the service did not put its signing key in the data directory: %v", err)
	}

	// stdio: the child, pointed at the service's database, accepts the service-issued key.
	line, _, err := h.child(key, "loom_status", map[string]any{})
	if err != nil || isToolError(line) {
		t.Fatalf("stdio child with a service-issued key: %v %s", err, line)
	}
	childKey := filepath.Join(h.workspaceDir(), "identity-signing.key")
	a, aErr := os.ReadFile(serviceKey) //nolint:gosec // a path inside this test's temp home
	b, bErr := os.ReadFile(childKey)   //nolint:gosec // a path inside this test's temp home
	if bErr != nil || aErr != nil {
		t.Fatalf("reading the signing keys: service %v, child (expected beside the database) %v", aErr, bErr)
	}
	if len(a) == 0 || bytes.Equal(a, b) {
		t.Fatal("expected two different signing keys (the finding); they are the same")
	}
	t.Logf("signing keys: service %s, child %s: different", serviceKey, childKey)

	// the same key on the service's /mcp
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
		t.Fatalf("service /mcp with the same key = %d %s", res.StatusCode, raw)
	}

	// a write through the child is visible through the service: one database
	line, _, err = h.child(key, "loom_template_put", map[string]any{"name": "from-child", "generator": "wiki_page", "body": "x"})
	if err != nil || isToolError(line) {
		t.Fatalf("child write: %v %s", err, line)
	}
	get, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://"+h.addr+"/api/templates/from-child", nil)
	get.Header.Set("Authorization", "Bearer "+key)
	res, err = http.DefaultClient.Do(get)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("the service does not see the child's write: %d", res.StatusCode)
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
	kid := h.command("mcp", "--no-project", "-db", h.db())
	kid.Stderr = &cl
	kid.Stdin = strings.NewReader("")
	h.writeConfig("") // no stdio token: the child ends with the token error once it has started
	var wg sync.WaitGroup
	start := make(chan struct{})
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

// (c), the first start: both processes meet an empty file and both run the migrations. This is
// NOT safe: one of them can lose the race and exit (database is locked, or the other already
// created the tables). The test pins down what is guaranteed instead: the file is never left
// damaged, and a lone start afterwards comes up. The cutover plan therefore never starts the two
// together on an empty file (migrate-data creates the schema first; the service starts before
// the child).
func TestConcurrentFirstStartLeavesAUsableFile(t *testing.T) {
	lost := 0
	const rounds = 5
	for round := range rounds {
		h := newHost(t)
		if err := os.MkdirAll(h.workspaceDir(), 0o750); err != nil {
			t.Fatal(err)
		}
		svc, svcLog, childLog := raceStart(t, h)
		survived := h.answers()
		stop(svc)
		childOK := strings.Contains(childLog, "needs a token")
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
	t.Logf("%d of %d concurrent first starts lost the race in at least one process", lost, rounds)
}

// (c), every later start: the file is already migrated (which migrate-data, or one earlier
// start, guarantees), and the identity key exists. Both processes may then start at the same
// moment and neither may fail.
func TestConcurrentStartOnMigratedFile(t *testing.T) {
	h := newHost(t)
	first := h.service()
	stop(first)
	for round := range 3 {
		svc, svcLog, childLog := raceStart(t, h)
		up := h.answers()
		stop(svc)
		if !up || !strings.Contains(childLog, "needs a token") {
			t.Fatalf("round %d: concurrent start on a migrated file failed (service up=%v)\nservice: %s\nchild: %s", round, up, svcLog, childLog)
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

// (d): a write through the child while another connection holds the write lock waits for it and
// succeeds.
func TestChildWriteWaitsForTheWriteLock(t *testing.T) {
	h := newHost(t)
	h.service()
	_, key := h.owner()

	put := func(name string) (string, time.Duration, error) {
		return h.child(key, "loom_template_put", map[string]any{"name": name, "generator": "wiki_page", "body": "x"})
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

	// A hold longer than the 5s busy timeout is still waited out: WriteTx restarts the transaction
	// on busy (up to 5 attempts), so the bound is about 25s, not 5s.
	<-holdWrite(t, h.db(), 7*time.Second)
	line, took, err = put("waited-longer")
	if err != nil || isToolError(line) {
		t.Fatalf("a write during a 7s lock failed: %v %s", err, line)
	}
	if took < 6*time.Second {
		t.Fatalf("the write took %s: it did not wait for the lock", took)
	}
	t.Logf("write during a 7s hold: waited %s and succeeded", took.Round(100*time.Millisecond))
}
