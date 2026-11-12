package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hollis-labs/loom/internal/directivex"
	"github.com/hollis-labs/loom/internal/settings"
)

func TestSplitAppArgs(t *testing.T) {
	cmdArgs, appArgs, err := splitAppArgs([]string{"list", "--db", "/tmp/loom.db", "-limit", "5", "--workspace=team", "--no-project"})
	if err != nil {
		t.Fatalf("splitAppArgs: %v", err)
	}
	wantCmd := []string{"list", "-limit", "5"}
	wantApp := []string{"-db", "/tmp/loom.db", "-workspace=team", "-project=false"}
	if !reflect.DeepEqual(cmdArgs, wantCmd) {
		t.Fatalf("cmd args = %+v, want %+v", cmdArgs, wantCmd)
	}
	if !reflect.DeepEqual(appArgs, wantApp) {
		t.Fatalf("app args = %+v, want %+v", appArgs, wantApp)
	}
}

func TestSplitAppArgsMissingValue(t *testing.T) {
	if _, _, err := splitAppArgs([]string{"list", "--db"}); err == nil {
		t.Fatalf("splitAppArgs missing db value returned nil error")
	}
}

func TestResolveLayoutSupportsNoProjectFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Chdir(t.TempDir())

	projectLayout, err := resolveLayout(nil)
	if err != nil {
		t.Fatalf("resolve project layout: %v", err)
	}
	if !projectLayout.ProjectMode() {
		t.Fatalf("default layout project mode = false")
	}

	xdgLayout, err := resolveLayout([]string{"--no-project"})
	if err != nil {
		t.Fatalf("resolve no-project layout: %v", err)
	}
	if xdgLayout.ProjectMode() {
		t.Fatalf("no-project layout project mode = true")
	}
	if xdgLayout.DataDir() != filepath.Join(home, "data", "loom") {
		t.Fatalf("no-project data dir = %q", xdgLayout.DataDir())
	}
}

func TestLoomRouteUsesBoundedLabels(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/", "/"},
		{"/assets/index.js", "/webui"},
		{"/mcp", "/mcp"},
		{"/api/status", "/api/status"},
		{"/api/bundles/nanite", "/api/bundles/{bundle}"},
		{"/api/bundles/team-wiki/pages", "/api/bundles/{bundle}/pages"},
		{"/api/bundles/team-wiki/compile-jobs", "/api/bundles/{bundle}/compile-jobs"},
		{"/api/bundles/team-wiki/export", "/api/bundles/{bundle}/export"},
		{"/api/bundles/team-wiki/conformance", "/api/bundles/{bundle}/conformance"},
		{"/api/pages/nanite/runtime", "/api/pages/{bundle}/{slug}"},
		{"/api/pages/nanite/runtime/links", "/api/pages/{bundle}/{slug}/links"},
		{"/api/pages/nanite/runtime/conformance", "/api/pages/{bundle}/{slug}/conformance"},
		{"/api/compile-jobs/123", "/api/compile-jobs/{id}"},
		{"/api/compile-jobs/123/events", "/api/compile-jobs/{id}/events"},
		{"/api/ingest/abcd1234", "/api/ingest/{hash}"},
		{"/api/ingest/text", "/api/ingest/text"},
		{"/api/directives/parse", "/api/directives/parse"},
		{"/api/directives/abcd1234", "/api/directives/{hash}"},
		{"/api/templates/wiki_page.default", "/api/templates/{name}"},
		{"/api/templates/wiki_page.default/render", "/api/templates/{name}/render"},
	}
	for _, tt := range tests {
		req, err := http.NewRequest(http.MethodGet, tt.path, nil)
		if err != nil {
			t.Fatalf("NewRequest %s: %v", tt.path, err)
		}
		if got := loomRoute(req); got != tt.want {
			t.Fatalf("loomRoute(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestDirectivesCLIParsesFromReader(t *testing.T) {
	var out bytes.Buffer
	err := directivesCLIWithIO([]string{"-source", "cli-test"}, strings.NewReader("::context_start Loom\nnotes\n::note Capture this\n::context_end"), &out)
	if err != nil {
		t.Fatalf("directivesCLIWithIO: %v", err)
	}
	var resp directivex.ParseResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Directives) != 1 {
		t.Fatalf("directives len = %d, want 1: %+v", len(resp.Directives), resp.Directives)
	}
	if resp.Directives[0].Command != "note" || resp.Directives[0].Prompt != "Capture this" || resp.Directives[0].Source != "cli-test" {
		t.Fatalf("directive = %+v", resp.Directives[0])
	}
}

func TestIngestFiltersUseSettingsAndExplicitOverrides(t *testing.T) {
	l := &loom{settings: settings.Settings{
		RedactPatterns: []string{`secret=\w+`},
		GitExclude:     []string{"node_modules", ".git"},
	}}

	excludes, redactions := l.ingestFilters(nil, nil)
	if !reflect.DeepEqual(excludes, []string{"node_modules", ".git"}) {
		t.Fatalf("excludes = %+v", excludes)
	}
	if !reflect.DeepEqual(redactions, []string{`secret=\w+`}) {
		t.Fatalf("redactions = %+v", redactions)
	}

	excludes, redactions = l.ingestFilters([]string{"vendor"}, []string{"token"})
	if !reflect.DeepEqual(excludes, []string{"vendor"}) {
		t.Fatalf("explicit excludes = %+v", excludes)
	}
	if !reflect.DeepEqual(redactions, []string{"token"}) {
		t.Fatalf("explicit redactions = %+v", redactions)
	}
}
