// router_test.go
package api

import (
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"

	"github.com/gorilla/mux"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/routes.golden")

// TestRoutesAreStable pins the REST API surface (method + path template). The
// frontend calls these routes, so a refactor must neither drop nor rename one by
// accident. After an intentional change run: go test -run TestRoutesAreStable -update
func TestRoutesAreStable(t *testing.T) {
	router := NewRouter(kube.NewServiceAccountProvider(nil), t.TempDir(), "test")
	var got []string
	err := router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		tmpl, err := route.GetPathTemplate()
		if err != nil {
			return nil // the PathPrefix("/api/v1") parent has no handler of its own
		}
		methods, _ := route.GetMethods()
		if len(methods) == 0 {
			methods = []string{"ANY"}
		}
		for _, m := range methods {
			got = append(got, m+" "+tmpl)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	out := strings.Join(got, "\n") + "\n"

	const golden = "testdata/routes.golden"
	if *updateGolden {
		if err := os.WriteFile(golden, []byte(out), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != out {
		t.Errorf("route table changed.\n--- golden\n%s--- now\n%s", want, out)
	}
}

// apiRoutes lists every method + path of the REST API, with {params} filled in.
func apiRoutes(t *testing.T, router *mux.Router) [][2]string {
	t.Helper()
	var out [][2]string
	err := router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		tmpl, err := route.GetPathTemplate()
		if err != nil || !strings.HasPrefix(tmpl, "/api/v1/") {
			return nil
		}
		methods, _ := route.GetMethods()
		for _, m := range methods {
			out = append(out, [2]string{m, regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(tmpl, "x")})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func tokenModeRouter(t *testing.T, uiDir string) *mux.Router {
	t.Helper()
	t.Setenv("USER_AUTH", "token")
	t.Setenv("KUBE_API_URL", "https://rancher.invalid/k8s/clusters/local")
	provider, err := kube.NewProvider()
	if err != nil {
		t.Fatal(err)
	}
	return NewRouter(provider, uiDir, "test")
}

// Per-user authorisation (Phase 2) depends on every API handler being wrapped by
// kube.Scoped. A route registered without it would run with no user token at all, so
// this walks the whole route table instead of trusting each registration by eye.
func TestEveryAPIRouteRequiresAUserToken(t *testing.T) {
	router := tokenModeRouter(t, t.TempDir())
	routes := apiRoutes(t, router)
	if len(routes) < 50 {
		t.Fatalf("only %d API routes found; the walk is not seeing the table", len(routes))
	}
	// The single deliberate exception: a browser download cannot send the token
	// header, so this route takes a signed ticket instead (export/download.go).
	// It must still refuse a request without a valid ticket.
	const ticketRoute = "GET /api/v1/exports/x/x/download" // {namespace}/{id} as the walk renders them
	sawTicketRoute := false
	for _, r := range routes {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest(r[0], r[1], nil))
		if r[0]+" "+r[1] == ticketRoute {
			sawTicketRoute = true
			if rr.Code != http.StatusUnauthorized || !strings.Contains(rr.Body.String(), "ticket") {
				t.Errorf("%s without a ticket: status %d, want 401 (%s)", ticketRoute, rr.Code, strings.TrimSpace(rr.Body.String()))
			}
			continue
		}
		if rr.Code != http.StatusUnauthorized || !strings.Contains(rr.Body.String(), "X-Migration-Token") {
			t.Errorf("%s %s without a token: status %d, want 401 (%s)", r[0], r[1], rr.Code, strings.TrimSpace(rr.Body.String()))
		}
	}
	if !sawTicketRoute {
		t.Errorf("%s is gone; update this exception list", ticketRoute)
	}
}

// The page itself must load without a token: the SPA is what mints one.
func TestStaticFrontendNeedsNoToken(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<title>ui</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "static"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "static", "app.js"), []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}
	router := tokenModeRouter(t, dir)
	for path, want := range map[string]int{"/": http.StatusOK, "/static/app.js": http.StatusOK, "/static/missing.js": http.StatusNotFound} {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest("GET", path, nil))
		if rr.Code != want {
			t.Errorf("GET %s: status %d, want %d", path, rr.Code, want)
		}
	}
}

func TestRouterRejectsTheWrongMethod(t *testing.T) {
	router := NewRouter(kube.NewServiceAccountProvider(nil), t.TempDir(), "test")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest("DELETE", "/api/v1/capabilities", nil))
	if rr.Code == http.StatusOK {
		t.Errorf("DELETE on a GET-only route answered %d", rr.Code)
	}
}
