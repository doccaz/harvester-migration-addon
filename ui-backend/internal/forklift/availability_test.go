// availability_test.go
package forklift

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"
	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
)

func host(ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "forklift.konveyor.io/v1beta1", "kind": "Provider",
		"metadata": map[string]interface{}{"name": "host", "namespace": ns},
	}}
}

func availability(t *testing.T, c *kube.Clients, query string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/forklift/availability"+query, nil)
	rr := httptest.NewRecorder()
	CheckAvailability(c)(rr, req)
	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("not JSON: %v: %s", err, rr.Body.String())
	}
	return rr.Code, body
}

// The page decides what to show from this one answer, so "absent", "not allowed" and
// "could not check" must stay distinguishable, and the old fields must stay put.
func TestCheckAvailability(t *testing.T) {
	missingCRD := apierrors.NewGenericServerResponse(404, "get", schema.GroupResource{Group: "forklift.konveyor.io", Resource: "providers"}, "",
		"the server could not find the requested resource", 0, false)

	cases := []struct {
		name      string
		clients   func() *kube.Clients
		query     string
		available bool
		state     string
		namespace string
		message   string
	}{
		{"available", func() *kube.Clients { return testutil.NewClientsWithDynamic(nil, host("forklift")) }, "", true, "available", "forklift", ""},
		{"other namespace", func() *kube.Clients { return testutil.NewClientsWithDynamic(nil, host("fk2")) }, "?namespace=fk2", true, "available", "fk2", ""},
		{"installed but not ready", func() *kube.Clients { return testutil.NewClientsWithDynamic(nil) }, "", false, "not-ready", "forklift",
			"host Provider not found in namespace forklift"},
		{"not installed", func() *kube.Clients {
			c := testutil.NewClientsWithDynamic(nil)
			testutil.Fail(c, "get", "providers", missingCRD)
			return c
		}, "", false, "not-installed", "forklift", "not installed"},
		{"no permission", func() *kube.Clients {
			c := testutil.NewClientsWithDynamic(nil, host("forklift"))
			testutil.Fail(c, "get", "providers", testutil.ErrForbidden())
			return c
		}, "", false, "forbidden", "forklift", "permission"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := availability(t, tc.clients(), tc.query)
			if code != http.StatusOK {
				t.Fatalf("status %d: the answer is data, not an error", code)
			}
			if body["available"] != tc.available || body["state"] != tc.state || body["defaultNamespace"] != tc.namespace {
				t.Errorf("body = %v", body)
			}
			msg, _ := body["message"].(string)
			if tc.message == "" && msg != "" || !strings.Contains(msg, tc.message) {
				t.Errorf("message = %q, want it to contain %q", msg, tc.message)
			}
		})
	}
}
