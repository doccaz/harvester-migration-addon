// rows.go
package testutil

import (
	"errors"
	"net/http"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Row is one case of a table-driven handler test: call Handler with Method, Body and
// route Vars, and expect HTTP status Want.
type Row struct {
	Name    string
	Handler http.HandlerFunc
	Method  string
	Body    interface{}
	Vars    map[string]string
	Want    int
}

// Case builds a Row; use it instead of an unkeyed Row literal, which go vet rejects
// for a type from another package.
func Case(name string, h http.HandlerFunc, method string, body interface{}, vars map[string]string, want int) Row {
	return Row{Name: name, Handler: h, Method: method, Body: body, Vars: vars, Want: want}
}

// Run executes every row as a subtest.
func Run(t *testing.T, rows []Row) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.Name, func(t *testing.T) {
			rr := Do(r.Handler, r.Method, "/x", r.Body, r.Vars)
			if rr.Code != r.Want {
				t.Errorf("status %d, want %d: %s", rr.Code, r.Want, rr.Body.String())
			}
		})
	}
}

var thing = schema.GroupResource{Resource: "things"}

// Kubernetes API errors for injecting failures with Fail.
func ErrNotFound() error      { return apierrors.NewNotFound(thing, "x") }
func ErrAlreadyExists() error { return apierrors.NewAlreadyExists(thing, "x") }
func ErrConflict() error      { return apierrors.NewConflict(thing, "x", errors.New("stale")) }
func ErrForbidden() error     { return apierrors.NewForbidden(thing, "x", errors.New("no")) }
