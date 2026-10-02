// status_test.go
package vmic

import (
	"errors"
	"net/http"
	"testing"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"
	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// These tests pin how each handler maps a failing Kubernetes call to an HTTP
// status: the API server's own status passes through (404, 409, 403, ...) and 500 is
// reserved for failures that are really the server's. Each row injects one failure.

const ns = "labs"

var gr = schema.GroupResource{Resource: "things"}

func errAlreadyExists() error { return apierrors.NewAlreadyExists(gr, "x") }
func errConflict() error      { return apierrors.NewConflict(gr, "x", errors.New("stale")) }
func errForbidden() error     { return apierrors.NewForbidden(gr, "x", errors.New("no")) }

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

type row struct {
	name    string
	handler http.HandlerFunc
	method  string
	body    interface{}
	vars    map[string]string
	want    int
}

func run(t *testing.T, rows []row) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			rr := testutil.Do(r.handler, r.method, "/x", r.body, r.vars)
			if rr.Code != r.want {
				t.Errorf("status %d, want %d: %s", rr.Code, r.want, rr.Body.String())
			}
		})
	}
}

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
	testutil.Fail(listFail, "list", "virtualmachineimports", errForbidden())

	run(t, []row{
		{"create: already exists is 409", CreatePlan(failing("create", "virtualmachineimports", errAlreadyExists())), "POST", createBody, nil, http.StatusConflict},
		{"create: forbidden is 403", CreatePlan(failing("create", "virtualmachineimports", errForbidden())), "POST", createBody, nil, http.StatusForbidden},
		{"list: forbidden is 403", ListPlans(listFail), "GET", nil, nil, http.StatusForbidden},
		{"delete: missing is 404", DeletePlan(withObjects(nil)), "DELETE", nil, vars, http.StatusNotFound},
		{"delete: forbidden is 403", DeletePlan(failing("delete", "virtualmachineimports", errForbidden(), importPlan("a"))), "DELETE", nil, vars, http.StatusForbidden},
		{"update: conflict is 409", UpdatePlan(failing("update", "virtualmachineimports", errConflict(), importPlan("a"))), "PUT", UpdatePlanPayload{}, vars, http.StatusConflict},
		{"run: missing plan is 404", RunPlan(withObjects(nil)), "POST", nil, vars, http.StatusNotFound},
		{"run: conflict is 409", RunPlan(failing("update", "virtualmachineimports", errConflict(), importPlan("a"))), "POST", nil, vars, http.StatusConflict},
		{"logs: missing plan is 404", GetPlanLogs(withObjects(nil)), "GET", nil, vars, http.StatusNotFound},
		{"logs: pod list forbidden is 403", GetPlanLogs(podsFail(errForbidden())), "GET", nil, vars, http.StatusForbidden},
		{"logs: no controller pod is 404", GetPlanLogs(withObjects(nil, importPlan("a"))), "GET", nil, vars, http.StatusNotFound},
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
			testutil.Fail(listFail, "list", k.resource, errForbidden())

			run(t, []row{
				{"list: forbidden is 403", k.list(listFail), "GET", nil, nil, http.StatusForbidden},
				{"create: secret already exists is 409", k.create(fail("create", "secrets", errAlreadyExists(), nil)), "POST", k.createBody, nil, http.StatusConflict},
				{"create: secret forbidden is 403", k.create(fail("create", "secrets", errForbidden(), nil)), "POST", k.createBody, nil, http.StatusForbidden},
				{"create: source already exists is 409", k.create(fail("create", k.resource, errAlreadyExists(), nil)), "POST", k.createBody, nil, http.StatusConflict},
				{"get: missing secret is 404", k.get(withObjects(nil, k.obj("a"))), "GET", nil, vars, http.StatusNotFound},
				{"update: missing secret is 404", k.update(withObjects(nil, k.obj("a"))), "PUT", k.updateBody, vars, http.StatusNotFound},
				{"update: secret conflict is 409", k.update(fail("update", "secrets", errConflict(), secrets, k.obj("a"))), "PUT", k.updateBody, vars, http.StatusConflict},
				{"update: source conflict is 409", k.update(fail("update", k.resource, errConflict(), secrets, k.obj("a"))), "PUT", k.updateBody, vars, http.StatusConflict},
				{"delete: missing source is 404", k.del(withObjects(nil)), "DELETE", nil, vars, http.StatusNotFound},
				{"delete: forbidden is 403", k.del(fail("delete", k.resource, errForbidden(), secrets, k.obj("a"))), "DELETE", nil, vars, http.StatusForbidden},
			})
		})
	}
}
