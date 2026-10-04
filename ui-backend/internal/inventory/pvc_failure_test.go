// pvc_failure_test.go
//
// "No PVCs" and "could not list PVCs" are different: the first is an empty map, the
// second used to be the same empty map, so a caller without cluster-wide PVC access saw
// disks of unknown size and, on export, a misleading "check that the claim exists".
package inventory

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"
	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
)

func inventoryClients() *kube.Clients {
	c := testutil.NewClientsWithListKinds(map[schema.GroupVersionResource]string{
		kube.VMGVR: "VirtualMachineList", kube.VMIKubevirtGVR: "VirtualMachineInstanceList",
	})
	return c
}

func TestPVCIndexReportsAFailedList(t *testing.T) {
	c := inventoryClients()
	testutil.Fail(c, "list", "persistentvolumeclaims", testutil.ErrForbidden())
	idx, err := PVCIndex(t.Context(), c)
	if err == nil {
		t.Fatal("a failed PVC list must be returned, not swallowed")
	}
	if len(idx) != 0 {
		t.Errorf("index = %v, want empty", idx)
	}

	idx, err = PVCIndex(t.Context(), inventoryClients())
	if err != nil || len(idx) != 0 {
		t.Errorf("no PVCs at all is an empty index and no error, got %v, %v", idx, err)
	}
}

func rootOf(t *testing.T, c *kube.Clients) Node {
	t.Helper()
	rr := testutil.Do(HandleGetHarvesterInventory(c), "GET", "/x", nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body)
	}
	var root Node
	if err := json.Unmarshal(rr.Body.Bytes(), &root); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestInventoryWarnsWhenItCouldNotReadEverything(t *testing.T) {
	t.Run("healthy: no warnings", func(t *testing.T) {
		if w := rootOf(t, inventoryClients()).Warnings; len(w) != 0 {
			t.Errorf("warnings = %v", w)
		}
	})
	t.Run("PVC list forbidden: still 200, with a warning that names the cause", func(t *testing.T) {
		c := inventoryClients()
		testutil.Fail(c, "list", "persistentvolumeclaims", testutil.ErrForbidden())
		w := rootOf(t, c).Warnings
		if len(w) != 1 || !strings.Contains(w[0], "PersistentVolumeClaims") || !strings.Contains(w[0], "forbidden") {
			t.Errorf("warnings = %v", w)
		}
	})
	t.Run("VMI list failing: every VM is treated as running, and the page says so", func(t *testing.T) {
		c := inventoryClients()
		testutil.Fail(c, "list", "virtualmachineinstances", errors.New("connection reset"))
		if _, err := c.Dynamic.Resource(kube.VMGVR).Namespace("dev").Create(t.Context(), inventoryVM("stopped-vm", "dev"), metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
		root := rootOf(t, c)
		if w := root.Warnings; len(w) != 1 || !strings.Contains(w[0], "VirtualMachineInstances") || !strings.Contains(w[0], "running") {
			t.Errorf("warnings = %v", w)
		}
		// Without VMI data the VM must fail closed: blocked from export as if running.
		vm := root.Children[0].Children[0]
		if !strings.Contains(strings.Join(vm.ExportBlockers, " "), "running") {
			t.Errorf("a VM whose run state is unknown must be blocked from export, blockers = %v", vm.ExportBlockers)
		}
	})
}
