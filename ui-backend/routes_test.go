// routes_test.go
package main

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"

	"github.com/gorilla/mux"
)

// updateGolden (-update) is declared in ovf_test.go and shared by all golden tests.
//
// TestRoutesAreStable pins the REST API surface (method + path template). The
// frontend calls these routes, so a refactor must neither drop nor rename one by
// accident. After an intentional change run: go test -run TestRoutesAreStable -update
func TestRoutesAreStable(t *testing.T) {
	router := newRouter(kube.NewServiceAccountProvider(nil), t.TempDir())
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
