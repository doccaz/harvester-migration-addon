// hidden_errors_test.go
//
// A read that fails for any reason used to be reported as 404 "not found" by these handlers, so
// a caller without permission, or an API server having a bad moment, was told the object did
// not exist. The status now follows the API server's: 404 only when the object is missing.
package vmic

import (
	"errors"
	"net/http"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"
	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
)

func TestReadsBeforeWritesKeepTheAPIStatus(t *testing.T) {
	cases := []struct {
		name     string
		resource string
		object   runtime.Object
		handler  func(*kube.Clients) http.HandlerFunc
		method   string
		body     interface{}
	}{
		{"get vCenter source", "vmwaresources", vmwareSource("a"), GetVmwareSource, http.MethodGet, nil},
		{"update vCenter source", "vmwaresources", vmwareSource("a"), UpdateVmwareSource, http.MethodPut, map[string]interface{}{}},
		{"get OVA source", "ovasources", ovaSource("a"), GetOvaSource, http.MethodGet, nil},
		{"update OVA source", "ovasources", ovaSource("a"), UpdateOvaSource, http.MethodPut, map[string]interface{}{}},
		{"update plan", "virtualmachineimports", importPlan("a"), UpdatePlan, http.MethodPut, map[string]interface{}{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status := func(c *kube.Clients) int {
				return testutil.Do(tc.handler(c), tc.method, "/x", tc.body, vars).Code
			}
			if got := status(withObjects(nil)); got != http.StatusNotFound {
				t.Errorf("missing object: %d, want 404", got)
			}
			forbidden := withObjects([]runtime.Object{secretObj("a-credentials")}, tc.object)
			testutil.Fail(forbidden, "get", tc.resource, testutil.ErrForbidden())
			if got := status(forbidden); got != http.StatusForbidden {
				t.Errorf("forbidden: %d, want 403 (not a 'not found')", got)
			}
			broken := withObjects([]runtime.Object{secretObj("a-credentials")}, tc.object)
			testutil.Fail(broken, "get", tc.resource, errors.New("connection reset"))
			if got := status(broken); got != http.StatusInternalServerError {
				t.Errorf("a failed call: %d, want 500", got)
			}
		})
	}
}
