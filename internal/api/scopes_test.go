package api_test

import (
	"net/http"
	"strings"
	"testing"

	libauth "github.com/hollis-labs/libs/security/auth"

	"github.com/hollis-labs/loom/internal/api"
)

// Every route the API serves has a scope in the table the guard enforces: reads need loom.read,
// everything else loom.write. Routes() is the single list the table is built from.
func TestEveryRouteHasItsScope(t *testing.T) {
	routes := api.Routes()
	if len(routes) == 0 {
		t.Fatal("no routes")
	}
	table := libauth.NewScopeTable()
	api.RegisterScopes(table)
	for _, rt := range routes {
		concrete := rt.Pattern
		for _, ph := range []string{"{slug}", "{bundle}", "{id}", "{hash}", "{name}"} {
			concrete = strings.ReplaceAll(concrete, ph, "x")
		}
		got, known := table.ForRoute(rt.Method, concrete)
		want := api.ScopeWrite
		if rt.Method == http.MethodGet {
			want = api.ScopeRead
		}
		if !known || got != want {
			t.Errorf("%s %s: scope=%q known=%v, want %q", rt.Method, concrete, got, known, want)
		}
		if !strings.HasPrefix(rt.Pattern, "/api/") {
			t.Errorf("%s %s is outside /api/, where the guard does not look", rt.Method, rt.Pattern)
		}
	}
	// A route nobody declared is unmapped, which the guard refuses in enforce mode.
	for _, undeclared := range [][2]string{{"GET", "/api/health"}, {"DELETE", "/api/bundles/x"}, {"GET", "/api/admin"}, {"PUT", "/api/pages/a/b"}} {
		if _, known := table.ForRoute(undeclared[0], undeclared[1]); known {
			t.Errorf("%s %s is mapped but not declared", undeclared[0], undeclared[1])
		}
	}
}
