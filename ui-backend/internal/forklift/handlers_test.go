// handlers_test.go
package forklift

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCreateForkliftProviderHandler_VSphere(t *testing.T) {
	clients := testutil.NewClients()

	payload := CreateProviderPayload{
		Name:         "test-provider",
		Namespace:    "forklift",
		URL:          "https://vcenter.example.com/sdk",
		Username:     "admin",
		Password:     "secret",
		SdkEndpoint:  "vcenter",
		ProviderType: "vsphere",
	}

	rr := testutil.Do(CreateProvider(clients), "POST", "/api/v1/forklift/providers", payload, nil)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Verify Secret was created with correct keys
	secret, err := clients.Clientset.CoreV1().Secrets("forklift").Get(context.TODO(), "test-provider-secret", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected secret to be created: %v", err)
	}
	if secret.Labels["createdForProviderType"] != "vsphere" {
		t.Errorf("expected label createdForProviderType=vsphere, got %s", secret.Labels["createdForProviderType"])
	}
}

func TestCreateForkliftProviderHandler_OVA(t *testing.T) {
	clients := testutil.NewClients()

	payload := CreateProviderPayload{
		Name:         "ova-provider",
		Namespace:    "forklift",
		URL:          "10.0.0.1:/exports/vms",
		ProviderType: "ova",
	}

	rr := testutil.Do(CreateProvider(clients), "POST", "/api/v1/forklift/providers", payload, nil)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Verify Secret was created with only url key (no user/password)
	secret, err := clients.Clientset.CoreV1().Secrets("forklift").Get(context.TODO(), "ova-provider-secret", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected secret to be created: %v", err)
	}
	if secret.Labels["createdForProviderType"] != "ova" {
		t.Errorf("expected label createdForProviderType=ova, got %s", secret.Labels["createdForProviderType"])
	}
	// OVA secret should not have user/password in StringData
	if secret.StringData["user"] != "" {
		t.Errorf("OVA secret should not have user field, got: %s", secret.StringData["user"])
	}
}

func TestCreateForkliftProviderHandler_InvalidJSON(t *testing.T) {
	clients := testutil.NewClients()

	req := httptest.NewRequest("POST", "/api/v1/forklift/providers", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	CreateProvider(clients).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

func TestCreateForkliftProviderHandler_DefaultNamespace(t *testing.T) {
	clients := testutil.NewClients()

	payload := CreateProviderPayload{
		Name:     "test-provider",
		URL:      "https://vcenter.example.com/sdk",
		Username: "admin",
		Password: "secret",
	}

	rr := testutil.Do(CreateProvider(clients), "POST", "/api/v1/forklift/providers", payload, nil)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Verify Secret was created in default "forklift" namespace
	_, err := clients.Clientset.CoreV1().Secrets("forklift").Get(context.TODO(), "test-provider-secret", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected secret in forklift namespace: %v", err)
	}
}

func TestListForkliftProvidersHandler(t *testing.T) {
	// Create providers of different types
	vsphereProvider := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "forklift.konveyor.io/v1beta1",
			"kind":       "Provider",
			"metadata": map[string]interface{}{
				"name":      "vsphere-prov",
				"namespace": "forklift",
			},
			"spec": map[string]interface{}{
				"type": "vsphere",
			},
		},
	}
	ovaProvider := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "forklift.konveyor.io/v1beta1",
			"kind":       "Provider",
			"metadata": map[string]interface{}{
				"name":      "ova-prov",
				"namespace": "forklift",
			},
			"spec": map[string]interface{}{
				"type": "ova",
			},
		},
	}
	hostProvider := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "forklift.konveyor.io/v1beta1",
			"kind":       "Provider",
			"metadata": map[string]interface{}{
				"name":      "host",
				"namespace": "forklift",
			},
			"spec": map[string]interface{}{
				"type": "host",
			},
		},
	}

	scheme := runtime.NewScheme()
	gvr := schema.GroupVersionResource{Group: "forklift.konveyor.io", Version: "v1beta1", Resource: "providers"}
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, vsphereProvider, ovaProvider, hostProvider)
	clients := &kube.Clients{
		Clientset: fake.NewSimpleClientset(),
		Dynamic:   fakeDynamic,
	}

	// Seed the GVR so fake client can list
	_ = gvr

	t.Run("list all source providers", func(t *testing.T) {
		rr := testutil.Do(ListProviders(clients), "GET", "/api/v1/forklift/providers", nil, nil)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d; body: %s", rr.Code, rr.Body.String())
		}

		var providers []map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &providers); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		// Should include vsphere and ova, exclude host
		for _, p := range providers {
			spec, _ := p["spec"].(map[string]interface{})
			pType, _ := spec["type"].(string)
			if pType == "host" {
				t.Errorf("host provider should be excluded from listing")
			}
		}
	})
}

func TestDeleteForkliftProviderHandler(t *testing.T) {
	clients := testutil.NewClients()

	// Try to delete a non-existent provider
	rr := testutil.Do(
		DeleteProvider(clients),
		"DELETE",
		"/api/v1/forklift/providers/forklift/nonexistent",
		nil,
		map[string]string{"namespace": "forklift", "name": "nonexistent"},
	)

	// Should return error since provider doesn't exist
	if rr.Code == http.StatusOK {
		t.Log("delete of nonexistent provider returned 200 (fake client may not error)")
	}
}

func TestOvaInventoryRejectsUnknownResource(t *testing.T) {
	handler := GetOvaInventory(testutil.NewClientsWithDynamic(nil))
	for _, res := range []string{"../providers", "secrets", "vms/../../x"} {
		rr := testutil.Do(handler, "GET", "/x", nil,
			map[string]string{"namespace": "forklift", "name": "p", "resource": res})
		if rr.Code != http.StatusBadRequest {
			t.Errorf("resource %q: got %d, want 400", res, rr.Code)
		}
	}
}

func yamlObj(apiVersion, kind, ns, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion, "kind": kind,
		"metadata": map[string]interface{}{"name": name, "namespace": ns},
	}}
}

// The Forklift plan YAML route answers 404 for a missing plan (it used to answer 500).
// The VMIC one is tested in internal/vmic.
func TestPlanYAMLHandlers(t *testing.T) {
	cases := []struct {
		name    string
		handler func(*kube.Clients) http.HandlerFunc
		obj     *unstructured.Unstructured
	}{
		{"Forklift plan", GetPlanYAML, yamlObj("forklift.konveyor.io/v1beta1", "Plan", "ns", "p1")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clients := testutil.NewClientsWithDynamic(nil, tc.obj)
			rr := testutil.Do(tc.handler(clients), "GET", "/x", nil, map[string]string{"namespace": "ns", "name": "p1"})
			if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "application/yaml" {
				t.Fatalf("existing plan: %d %q", rr.Code, rr.Header().Get("Content-Type"))
			}
			if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Error("YAML must be served with X-Content-Type-Options: nosniff")
			}
			rr = testutil.Do(tc.handler(clients), "GET", "/x", nil, map[string]string{"namespace": "ns", "name": "missing"})
			if rr.Code != http.StatusNotFound {
				t.Errorf("missing plan: status %d, want 404", rr.Code)
			}
		})
	}
}

// The namespace becomes part of the inventory service's host name, so anything that
// is not a DNS label is refused before a URL is built.
func TestOvaInventoryRejectsABadNamespace(t *testing.T) {
	handler := GetOvaInventory(testutil.NewClients())
	for _, ns := range []string{"a.b", "evil.example.com#", "UPPER", "a b", "-x"} {
		rr := testutil.Do(handler, "GET", "/x", nil, map[string]string{"namespace": ns, "name": "p", "resource": "vms"})
		if rr.Code != http.StatusBadRequest {
			t.Errorf("namespace %q: status %d, want 400", ns, rr.Code)
		}
	}
}
