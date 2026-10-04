// hidden_errors_test.go
package forklift

import (
	"errors"
	"net/http"
	"testing"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"
	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
)

// Reading the Provider first must not turn a permission problem or an API failure into
// "not found": the status follows the API server's, and only a missing object is 404.
func TestProviderReadsKeepTheAPIStatus(t *testing.T) {
	cases := []struct {
		name    string
		handler func(*kube.Clients) http.HandlerFunc
		method  string
		body    interface{}
		vars    map[string]string
	}{
		{"get provider", GetProvider, http.MethodGet, nil, map[string]string{"namespace": "forklift", "name": "host"}},
		{"update provider", UpdateProvider, http.MethodPut, map[string]interface{}{}, map[string]string{"namespace": "forklift", "name": "host"}},
		{"OVA inventory", GetOvaInventory, http.MethodGet, nil, map[string]string{"namespace": "forklift", "name": "host", "resource": "vms"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status := func(c *kube.Clients) int {
				return testutil.Do(tc.handler(c), tc.method, "/x", tc.body, tc.vars).Code
			}
			if got := status(testutil.NewClientsWithDynamic(nil)); got != http.StatusNotFound {
				t.Errorf("missing provider: %d, want 404", got)
			}
			forbidden := testutil.NewClientsWithDynamic(nil, host("forklift"))
			testutil.Fail(forbidden, "get", "providers", testutil.ErrForbidden())
			if got := status(forbidden); got != http.StatusForbidden {
				t.Errorf("forbidden: %d, want 403 (not a 'not found')", got)
			}
			broken := testutil.NewClientsWithDynamic(nil, host("forklift"))
			testutil.Fail(broken, "get", "providers", errors.New("connection reset"))
			if got := status(broken); got != http.StatusInternalServerError {
				t.Errorf("a failed call: %d, want 500", got)
			}
		})
	}
}
