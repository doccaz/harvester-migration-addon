// status_test.go
package forklift

import (
	"net/http"
	"testing"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"
	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// These tests pin how each handler maps a failing Kubernetes call to an HTTP status:
// the API server's own status passes through (404, 409, 403, ...) and 500 is
// reserved for failures that are really the server's. Each row injects one failure.

const ns = "forklift"

var vars = map[string]string{"namespace": ns, "name": "a"}

func secretObj(name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Data:       map[string][]byte{"user": []byte("u"), "password": []byte("p")},
	}
}

func fl(kind, name string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "forklift.konveyor.io/v1beta1", "kind": kind,
		"metadata": map[string]interface{}{"name": name, "namespace": ns},
		"spec":     spec,
	}}
}

func provider(name string) *unstructured.Unstructured {
	return fl("Provider", name, map[string]interface{}{
		"type": "vsphere", "url": "https://vc.example.com/sdk",
		"secret": map[string]interface{}{"name": name + "-secret", "namespace": ns},
	})
}

func plan(name string) *unstructured.Unstructured { return fl("Plan", name, map[string]interface{}{}) }

// emptyClients can LIST every Forklift resource these handlers use, with no objects.
func emptyClients() *kube.Clients {
	return testutil.NewClientsWithListKinds(map[schema.GroupVersionResource]string{
		kube.ForkliftProviderGVR:  "ProviderList",
		kube.ForkliftPlanGVR:      "PlanList",
		kube.ForkliftMigrationGVR: "MigrationList",
	})
}

func withObjects(core []runtime.Object, dyn ...runtime.Object) *kube.Clients {
	return testutil.NewClientsWithDynamic(core, dyn...)
}

// failing returns clients seeded with objects where every verb on resource fails.
func failing(verb, resource string, err error, core []runtime.Object, dyn ...runtime.Object) *kube.Clients {
	c := withObjects(core, dyn...)
	testutil.Fail(c, verb, resource, err)
	return c
}

func listFailing(resource string) *kube.Clients {
	c := emptyClients()
	testutil.Fail(c, "list", resource, testutil.ErrForbidden())
	return c
}

func TestProviderStatuses(t *testing.T) {
	create := CreateProviderPayload{Name: "a", Namespace: ns, URL: "https://vc.example.com/sdk", Username: "u", Password: "p", SdkEndpoint: "vcenter", ProviderType: "vsphere"}
	update := CreateProviderPayload{Username: "u2", Password: "p2"}
	secrets := []runtime.Object{secretObj("a-secret")}

	testutil.Run(t, []testutil.Row{
		testutil.Case("list: forbidden is 403", ListProviders(listFailing("providers")), "GET", nil, nil, http.StatusForbidden),
		testutil.Case("create: secret already exists is 409", CreateProvider(failing("create", "secrets", testutil.ErrAlreadyExists(), nil)), "POST", create, nil, http.StatusConflict),
		testutil.Case("create: secret forbidden is 403", CreateProvider(failing("create", "secrets", testutil.ErrForbidden(), nil)), "POST", create, nil, http.StatusForbidden),
		testutil.Case("create: provider already exists is 409", CreateProvider(failing("create", "providers", testutil.ErrAlreadyExists(), nil)), "POST", create, nil, http.StatusConflict),
		testutil.Case("update: missing secret is 404", UpdateProvider(withObjects(nil, provider("a"))), "PUT", update, vars, http.StatusNotFound),
		testutil.Case("update: secret conflict is 409", UpdateProvider(failing("update", "secrets", testutil.ErrConflict(), secrets, provider("a"))), "PUT", update, vars, http.StatusConflict),
		testutil.Case("update: provider conflict is 409", UpdateProvider(failing("update", "providers", testutil.ErrConflict(), secrets, provider("a"))), "PUT", update, vars, http.StatusConflict),
		testutil.Case("delete: missing provider is 404", DeleteProvider(withObjects(nil)), "DELETE", nil, vars, http.StatusNotFound),
		testutil.Case("delete: forbidden is 403", DeleteProvider(failing("delete", "providers", testutil.ErrForbidden(), secrets, provider("a"))), "DELETE", nil, vars, http.StatusForbidden),
	})
}

func TestPlanStatuses(t *testing.T) {
	body := CreatePlanPayload{Name: "a", Namespace: ns, ProviderName: "p", ProviderNamespace: ns, TargetNamespace: "default"}

	testutil.Run(t, []testutil.Row{
		testutil.Case("list: forbidden is 403", ListPlans(listFailing("plans")), "GET", nil, nil, http.StatusForbidden),
		testutil.Case("create: network map already exists is 409", CreatePlan(failing("create", "networkmaps", testutil.ErrAlreadyExists(), nil)), "POST", body, nil, http.StatusConflict),
		testutil.Case("create: storage map already exists is 409", CreatePlan(failing("create", "storagemaps", testutil.ErrAlreadyExists(), nil)), "POST", body, nil, http.StatusConflict),
		testutil.Case("create: plan already exists is 409", CreatePlan(failing("create", "plans", testutil.ErrAlreadyExists(), nil)), "POST", body, nil, http.StatusConflict),
		testutil.Case("delete: missing plan is 404", DeletePlan(withObjects(nil)), "DELETE", nil, vars, http.StatusNotFound),
		testutil.Case("delete: forbidden is 403", DeletePlan(failing("delete", "plans", testutil.ErrForbidden(), nil, plan("a"))), "DELETE", nil, vars, http.StatusForbidden),
	})
}

func TestMigrationStatuses(t *testing.T) {
	testutil.Run(t, []testutil.Row{
		testutil.Case("create: already exists is 409", CreateMigration(failing("create", "migrations", testutil.ErrAlreadyExists(), nil)), "POST", nil, vars, http.StatusConflict),
		testutil.Case("create: forbidden is 403", CreateMigration(failing("create", "migrations", testutil.ErrForbidden(), nil)), "POST", nil, vars, http.StatusForbidden),
		testutil.Case("delete: forbidden is 403", DeleteMigration(failing("delete", "migrations", testutil.ErrForbidden(), nil)), "DELETE", nil, vars, http.StatusForbidden),
		testutil.Case("status: list forbidden is 403", GetMigrationStatus(listFailing("migrations")), "GET", nil, vars, http.StatusForbidden),
	})
}
