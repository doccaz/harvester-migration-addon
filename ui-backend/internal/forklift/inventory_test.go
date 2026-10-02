// inventory_test.go
package forklift

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
	"github.com/vmware/govmomi/simulator"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

func simulatedProvider(t *testing.T, mutate func(*unstructured.Unstructured)) http.HandlerFunc {
	t.Helper()
	model := simulator.VPX()
	t.Cleanup(model.Remove)
	if err := model.Create(); err != nil {
		t.Fatal(err)
	}
	s := model.Service.NewServer()
	t.Cleanup(s.Close)
	p := provider("a")
	_ = unstructured.SetNestedField(p.Object, s.URL.Scheme+"://"+s.URL.Host+s.URL.Path, "spec", "url")
	if mutate != nil {
		mutate(p)
	}
	return GetInventory(withObjects([]runtime.Object{secretObj("a-secret")}, p))
}

func TestGetInventory(t *testing.T) {
	t.Run("reads the tree through the provider's credentials", func(t *testing.T) {
		rr := testutil.Do(simulatedProvider(t, nil), "GET", "/x", nil, vars)
		if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "DC0_H0_VM0") {
			t.Errorf("status %d: %.200s", rr.Code, rr.Body)
		}
	})
	testutil.Run(t, []testutil.Row{
		testutil.Case("missing provider is 404", GetInventory(withObjects(nil)), "GET", nil, vars, http.StatusNotFound),
		testutil.Case("missing credentials secret is 404", GetInventory(withObjects(nil, provider("a"))), "GET", nil, vars, http.StatusNotFound),
		testutil.Case("a forbidden secret is 403", GetInventory(failing("get", "secrets", testutil.ErrForbidden(), []runtime.Object{secretObj("a-secret")}, provider("a"))), "GET", nil, vars, http.StatusForbidden),
		// Pinned: a malformed stored provider and an unreachable vCenter are the
		// server's problem and stay 500 (see docs/refactor-notes.md).
		testutil.Case("a provider without a URL is 500", GetInventory(withObjects([]runtime.Object{secretObj("a-secret")}, func() *unstructured.Unstructured {
			p := provider("a")
			unstructured.RemoveNestedField(p.Object, "spec", "url")
			return p
		}())), "GET", nil, vars, http.StatusInternalServerError),
		testutil.Case("an unreachable vCenter is 500", func() http.HandlerFunc {
			p := provider("a")
			_ = unstructured.SetNestedField(p.Object, "http://127.0.0.1:1/sdk", "spec", "url")
			return GetInventory(withObjects([]runtime.Object{secretObj("a-secret")}, p))
		}(), "GET", nil, vars, http.StatusInternalServerError),
	})
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// stubInventory replaces the package's HTTP client for one test and records the URL.
func stubInventory(t *testing.T, fn roundTripper) *string {
	t.Helper()
	orig := inventoryClient
	var got string
	inventoryClient = &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
		got = r.URL.String()
		return fn(r)
	})}
	t.Cleanup(func() { inventoryClient = orig })
	return &got
}

func inventoryService(ns string, port int32) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "forklift-inventory", Namespace: ns},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "api", Port: port}}},
	}
}

func ovaProvider() *unstructured.Unstructured {
	p := fl("Provider", "a", map[string]interface{}{"type": "ova"})
	p.SetUID(types.UID("uid-123"))
	return p
}

func TestOvaInventoryProxy(t *testing.T) {
	ovaVars := map[string]string{"namespace": ns, "name": "a", "resource": "networks"}

	t.Run("relays the inventory service's answer", func(t *testing.T) {
		url := stubInventory(t, func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`[{"id":"n1"}]`)), Header: http.Header{}}, nil
		})
		clients := withObjects([]runtime.Object{inventoryService(ns, 8443)}, ovaProvider())
		rr := testutil.Do(GetOvaInventory(clients), "GET", "/x", nil, ovaVars)
		if rr.Code != http.StatusOK || rr.Body.String() != `[{"id":"n1"}]` || rr.Header().Get("Content-Type") != "application/json" {
			t.Errorf("status %d body %q ct %q", rr.Code, rr.Body, rr.Header().Get("Content-Type"))
		}
		if *url != "http://forklift-inventory.forklift.svc:8443/providers/ova/uid-123/networks" {
			t.Errorf("proxied URL = %q", *url)
		}
	})
	t.Run("uses the port the service advertises", func(t *testing.T) {
		url := stubInventory(t, func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`[]`)), Header: http.Header{}}, nil
		})
		clients := withObjects([]runtime.Object{inventoryService(ns, 9443)}, ovaProvider())
		testutil.Do(GetOvaInventory(clients), "GET", "/x", nil, ovaVars)
		if !strings.Contains(*url, ":9443/") {
			t.Errorf("proxied URL = %q", *url)
		}
	})
	t.Run("passes an upstream error status through", func(t *testing.T) {
		stubInventory(t, func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(`{}`)), Header: http.Header{}}, nil
		})
		clients := withObjects([]runtime.Object{inventoryService(ns, 8443)}, ovaProvider())
		if rr := testutil.Do(GetOvaInventory(clients), "GET", "/x", nil, ovaVars); rr.Code != http.StatusNotFound {
			t.Errorf("status %d", rr.Code)
		}
	})
	t.Run("an unreachable inventory service is 502", func(t *testing.T) {
		stubInventory(t, func(*http.Request) (*http.Response, error) { return nil, errors.New("connection refused") })
		clients := withObjects([]runtime.Object{inventoryService(ns, 8443)}, ovaProvider())
		if rr := testutil.Do(GetOvaInventory(clients), "GET", "/x", nil, ovaVars); rr.Code != http.StatusBadGateway {
			t.Errorf("status %d", rr.Code)
		}
	})

	testutil.Run(t, []testutil.Row{
		testutil.Case("missing provider is 404", GetOvaInventory(withObjects(nil)), "GET", nil, ovaVars, http.StatusNotFound),
		// Used to be a 500: the inventory service simply is not installed.
		testutil.Case("no inventory service is 404", GetOvaInventory(withObjects(nil, ovaProvider())), "GET", nil, ovaVars, http.StatusNotFound),
	})
}
