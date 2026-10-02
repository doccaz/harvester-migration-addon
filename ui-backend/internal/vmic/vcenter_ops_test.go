// vcenter_ops_test.go
package vmic

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/inventory"
	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"
	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
	"github.com/vmware/govmomi/simulator"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// simulatedVCenter starts govmomi's in-process vCenter and returns clients that
// hold a VmwareSource pointing at it (plus its credentials Secret).
func simulatedVCenter(t *testing.T) (clients *kube.Clients, endpoint string) {
	t.Helper()
	model := simulator.VPX()
	t.Cleanup(model.Remove)
	if err := model.Create(); err != nil {
		t.Fatal(err)
	}
	s := model.Service.NewServer()
	t.Cleanup(s.Close)
	endpoint = s.URL.Scheme + "://" + s.URL.Host + s.URL.Path
	return clientsFor(endpoint), endpoint
}

func clientsFor(endpoint string) *kube.Clients {
	src := vmwareSource("a")
	_ = unstructured.SetNestedField(src.Object, endpoint, "spec", "endpoint")
	return testutil.NewClientsWithDynamic([]runtime.Object{secretObj("a-credentials")}, src)
}

func TestGetInventory(t *testing.T) {
	clients, _ := simulatedVCenter(t)
	rr := testutil.Do(GetInventory(clients), "GET", "/x", nil, vars)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body)
	}
	var root inventory.Node
	if err := json.Unmarshal(rr.Body.Bytes(), &root); err != nil || root.Name != "DC0" {
		t.Fatalf("tree root %q, err %v", root.Name, err)
	}

	t.Run("missing source is 404", func(t *testing.T) {
		rr := testutil.Do(GetInventory(testutil.NewClientsWithDynamic(nil)), "GET", "/x", nil, vars)
		if rr.Code != http.StatusNotFound {
			t.Errorf("status %d", rr.Code)
		}
	})
	t.Run("an unknown datacenter is 404", func(t *testing.T) {
		_, endpoint := simulatedVCenter(t)
		src := vmwareSource("a")
		_ = unstructured.SetNestedField(src.Object, endpoint, "spec", "endpoint")
		_ = unstructured.SetNestedField(src.Object, "nowhere", "spec", "dc")
		c := testutil.NewClientsWithDynamic([]runtime.Object{secretObj("a-credentials")}, src)
		if rr := testutil.Do(GetInventory(c), "GET", "/x", nil, vars); rr.Code != http.StatusNotFound {
			t.Errorf("status %d: %s", rr.Code, rr.Body)
		}
	})
}

func TestVCenterOperationStatuses(t *testing.T) {
	clients, _ := simulatedVCenter(t)
	power := func(vm, op string) interface{} { return VirtualMachinePowerRequest{VMName: vm, Operation: op} }

	testutil.Run(t, []testutil.Row{
		testutil.Case("power off", PowerOp(clients), "POST", power("DC0_H0_VM0", "off"), vars, http.StatusOK),
		testutil.Case("power on", PowerOp(clients), "POST", power("DC0_H0_VM0", "on"), vars, http.StatusOK),
		testutil.Case("unsupported operation is 400", PowerOp(clients), "POST", power("DC0_H0_VM0", "explode"), vars, http.StatusBadRequest),
		testutil.Case("unknown VM is 404", PowerOp(clients), "POST", power("no-such-vm", "on"), vars, http.StatusNotFound),
		testutil.Case("malformed body is 400", PowerOp(clients), "POST", "{not json", vars, http.StatusBadRequest),
		testutil.Case("rename", RenameVM(clients), "POST", VirtualMachineRenameRequest{OldName: "DC0_H0_VM1", NewName: "renamed"}, vars, http.StatusOK),
		testutil.Case("rename of an unknown VM is 404", RenameVM(clients), "POST", VirtualMachineRenameRequest{OldName: "no-such-vm", NewName: "x"}, vars, http.StatusNotFound),
		testutil.Case("unknown device key is 404", UpdateMAC(clients), "POST", UpdateVMMACRequest{VMName: "DC0_H0_VM0", DeviceKey: 99999, NewMAC: "00:50:56:aa:bb:cc"}, vars, http.StatusNotFound),
	})

	// Resolving the source goes through the Kubernetes API, so its statuses pass
	// through for all three handlers.
	missingSource := testutil.NewClientsWithDynamic(nil)
	missingSecret := testutil.NewClientsWithDynamic(nil, vmwareSource("a"))
	for name, h := range map[string]func(*kube.Clients) http.HandlerFunc{"power": PowerOp, "rename": RenameVM, "mac": UpdateMAC} {
		body := power("DC0_H0_VM0", "on")
		switch name {
		case "rename":
			body = VirtualMachineRenameRequest{OldName: "a", NewName: "b"}
		case "mac":
			body = UpdateVMMACRequest{VMName: "a", DeviceKey: 1, NewMAC: "00:50:56:aa:bb:cc"}
		}
		testutil.Run(t, []testutil.Row{
			testutil.Case(name+": missing source is 404", h(missingSource), "POST", body, vars, http.StatusNotFound),
			testutil.Case(name+": missing credentials secret is 404", h(missingSecret), "POST", body, vars, http.StatusNotFound),
		})
	}

	// Pinned on purpose: a malformed stored source and an unreachable vCenter are
	// the server's problem, so they stay 500 (see respondWithVCenterError).
	noEndpoint := vmwareSource("a")
	unstructured.RemoveNestedField(noEndpoint.Object, "spec", "endpoint")
	testutil.Run(t, []testutil.Row{
		testutil.Case("a source without an endpoint is 500", PowerOp(testutil.NewClientsWithDynamic([]runtime.Object{secretObj("a-credentials")}, noEndpoint)), "POST", power("x", "on"), vars, http.StatusInternalServerError),
		testutil.Case("an unreachable vCenter is 500", PowerOp(clientsFor("http://127.0.0.1:1/sdk")), "POST", power("x", "on"), vars, http.StatusInternalServerError),
	})
}
