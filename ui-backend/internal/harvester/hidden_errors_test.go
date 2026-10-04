// hidden_errors_test.go
package harvester

import (
	"errors"
	"net/http"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"
	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
)

func networkMap() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "forklift.konveyor.io/v1beta1", "kind": "NetworkMap",
		"metadata": map[string]interface{}{"name": "nm", "namespace": "forklift"},
	}}
}

// GetResource used to answer 404 for every failure; the status now follows the API server's.
func TestGetResourceKeepsTheAPIStatus(t *testing.T) {
	vars := map[string]string{"namespace": "forklift", "name": "nm"}
	status := func(c *kube.Clients) int {
		return testutil.Do(GetResource(c, kube.ForkliftNetworkMapGVR), http.MethodGet, "/x", nil, vars).Code
	}
	if got := status(testutil.NewClientsWithDynamic(nil)); got != http.StatusNotFound {
		t.Errorf("missing: %d, want 404", got)
	}
	forbidden := testutil.NewClientsWithDynamic(nil, networkMap())
	testutil.Fail(forbidden, "get", "networkmaps", testutil.ErrForbidden())
	if got := status(forbidden); got != http.StatusForbidden {
		t.Errorf("forbidden: %d, want 403", got)
	}
	broken := testutil.NewClientsWithDynamic(nil, networkMap())
	testutil.Fail(broken, "get", "networkmaps", errors.New("connection reset"))
	if got := status(broken); got != http.StatusInternalServerError {
		t.Errorf("a failed call: %d, want 500", got)
	}
}
