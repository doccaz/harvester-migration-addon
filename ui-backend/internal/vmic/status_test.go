// status_test.go
package vmic

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

// These tests pin how each handler maps a failing Kubernetes call to an HTTP
// status: the API server's own status passes through (404, 409, 403, ...) and 500 is
// reserved for failures that are really the server's. Each row injects one failure.

const ns = "labs"

func secretObj(name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Data:       map[string][]byte{"username": []byte("u"), "password": []byte("p")},
	}
}

func crd(apiVersion, kind, name string, spec map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion, "kind": kind,
		"metadata": map[string]interface{}{"name": name, "namespace": ns},
		"spec":     spec,
	}}
}

func vmwareSource(name string) *unstructured.Unstructured {
	return crd("migration.harvesterhci.io/v1beta1", "VmwareSource", name, map[string]interface{}{
		"endpoint": "https://vc.example.com/sdk", "dc": "DC0",
		"credentials": map[string]interface{}{"name": name + "-credentials", "namespace": ns},
	})
}

func ovaSource(name string) *unstructured.Unstructured {
	return crd("migration.harvesterhci.io/v1beta1", "OvaSource", name, map[string]interface{}{
		"url":         "http://files.example.com/vm.ova",
		"credentials": map[string]interface{}{"name": name + "-credentials", "namespace": ns},
	})
}

func importPlan(name string) *unstructured.Unstructured {
	return crd("migration.harvesterhci.io/v1beta1", "VirtualMachineImport", name, map[string]interface{}{"virtualMachineName": "vm"})
}

// emptyClients can LIST every resource these handlers use, with no objects.
func emptyClients() *kube.Clients {
	return testutil.NewClientsWithListKinds(map[schema.GroupVersionResource]string{
		kube.VMIGVR:          "VirtualMachineImportList",
		kube.VMwareSourceGVR: "VmwareSourceList",
		kube.OVASourceGVR:    "OvaSourceList",
	})
}

func withObjects(core []runtime.Object, dyn ...runtime.Object) *kube.Clients {
	return testutil.NewClientsWithDynamic(core, dyn...)
}

var vars = map[string]string{"namespace": ns, "name": "a"}

func TestPlanStatuses(t *testing.T) {
	createBody := map[string]interface{}{"metadata": map[string]interface{}{"name": "a", "namespace": ns}}
	failing := func(verb, res string, err error, objs ...runtime.Object) *kube.Clients {
		c := withObjects(nil, objs...)
		testutil.Fail(c, verb, res, err)
		return c
	}
	podsFail := func(err error) *kube.Clients {
		c := withObjects(nil, importPlan("a"))
		testutil.Fail(c, "list", "pods", err)
		return c
	}
	listFail := emptyClients()
	testutil.Fail(listFail, "list", "virtualmachineimports", testutil.ErrForbidden())

	testutil.Run(t, []testutil.Row{
		testutil.Case("create: already exists is 409", CreatePlan(failing("create", "virtualmachineimports", testutil.ErrAlreadyExists())), "POST", createBody, nil, http.StatusConflict),
		testutil.Case("create: forbidden is 403", CreatePlan(failing("create", "virtualmachineimports", testutil.ErrForbidden())), "POST", createBody, nil, http.StatusForbidden),
		testutil.Case("list: forbidden is 403", ListPlans(listFail), "GET", nil, nil, http.StatusForbidden),
		testutil.Case("delete: missing is 404", DeletePlan(withObjects(nil)), "DELETE", nil, vars, http.StatusNotFound),
		testutil.Case("delete: forbidden is 403", DeletePlan(failing("delete", "virtualmachineimports", testutil.ErrForbidden(), importPlan("a"))), "DELETE", nil, vars, http.StatusForbidden),
		testutil.Case("update: conflict is 409", UpdatePlan(failing("update", "virtualmachineimports", testutil.ErrConflict(), importPlan("a"))), "PUT", UpdatePlanPayload{}, vars, http.StatusConflict),
		testutil.Case("run: missing plan is 404", RunPlan(withObjects(nil)), "POST", nil, vars, http.StatusNotFound),
		testutil.Case("run: conflict is 409", RunPlan(failing("update", "virtualmachineimports", testutil.ErrConflict(), importPlan("a"))), "POST", nil, vars, http.StatusConflict),
		testutil.Case("logs: missing plan is 404", GetPlanLogs(withObjects(nil)), "GET", nil, vars, http.StatusNotFound),
		testutil.Case("logs: pod list forbidden is 403", GetPlanLogs(podsFail(testutil.ErrForbidden())), "GET", nil, vars, http.StatusForbidden),
		testutil.Case("logs: no controller pod is 404", GetPlanLogs(withObjects(nil, importPlan("a"))), "GET", nil, vars, http.StatusNotFound),
	})
}

// kinds holds what differs between the VMware and OVA source handlers.
type kind struct {
	name                           string
	list, create, get, update, del func(*kube.Clients) http.HandlerFunc
	obj                            func(string) *unstructured.Unstructured
	resource                       string
	createBody, updateBody         interface{}
}

func kinds() []kind {
	return []kind{
		{"vmware", ListVmwareSources, CreateVmwareSource, GetVmwareSource, UpdateVmwareSource, DeleteVmwareSource, vmwareSource, "vmwaresources",
			CreateVmwareSourcePayload{Name: "a", Namespace: ns, Endpoint: "https://vc/sdk", Datacenter: "DC0", Username: "u", Password: "p"},
			CreateVmwareSourcePayload{Endpoint: "https://vc2/sdk", Username: "u2", Password: "p2"}},
		{"ova", ListOvaSources, CreateOvaSource, GetOvaSource, UpdateOvaSource, DeleteOvaSource, ovaSource, "ovasources",
			CreateOvaSourcePayload{Name: "a", Namespace: ns, URL: "http://files/vm.ova", Username: "u", Password: "p"},
			CreateOvaSourcePayload{URL: "http://files/other.ova", Username: "u2", Password: "p2"}},
	}
}

func TestSourceStatuses(t *testing.T) {
	for _, k := range kinds() {
		t.Run(k.name, func(t *testing.T) {
			fail := func(verb, res string, err error, core []runtime.Object, dyn ...runtime.Object) *kube.Clients {
				c := withObjects(core, dyn...)
				testutil.Fail(c, verb, res, err)
				return c
			}
			secrets := []runtime.Object{secretObj("a-credentials")}
			listFail := emptyClients()
			testutil.Fail(listFail, "list", k.resource, testutil.ErrForbidden())

			testutil.Run(t, []testutil.Row{
				testutil.Case("list: forbidden is 403", k.list(listFail), "GET", nil, nil, http.StatusForbidden),
				testutil.Case("create: secret already exists is 409", k.create(fail("create", "secrets", testutil.ErrAlreadyExists(), nil)), "POST", k.createBody, nil, http.StatusConflict),
				testutil.Case("create: secret forbidden is 403", k.create(fail("create", "secrets", testutil.ErrForbidden(), nil)), "POST", k.createBody, nil, http.StatusForbidden),
				testutil.Case("create: source already exists is 409", k.create(fail("create", k.resource, testutil.ErrAlreadyExists(), nil)), "POST", k.createBody, nil, http.StatusConflict),
				testutil.Case("get: missing secret is 404", k.get(withObjects(nil, k.obj("a"))), "GET", nil, vars, http.StatusNotFound),
				testutil.Case("update: missing secret is 404", k.update(withObjects(nil, k.obj("a"))), "PUT", k.updateBody, vars, http.StatusNotFound),
				testutil.Case("update: secret conflict is 409", k.update(fail("update", "secrets", testutil.ErrConflict(), secrets, k.obj("a"))), "PUT", k.updateBody, vars, http.StatusConflict),
				testutil.Case("update: source conflict is 409", k.update(fail("update", k.resource, testutil.ErrConflict(), secrets, k.obj("a"))), "PUT", k.updateBody, vars, http.StatusConflict),
				testutil.Case("delete: missing source is 404", k.del(withObjects(nil)), "DELETE", nil, vars, http.StatusNotFound),
				testutil.Case("delete: forbidden is 403", k.del(fail("delete", k.resource, testutil.ErrForbidden(), secrets, k.obj("a"))), "DELETE", nil, vars, http.StatusForbidden),
			})
		})
	}
}
