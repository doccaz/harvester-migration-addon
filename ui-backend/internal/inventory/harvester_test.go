// harvester_test.go
package inventory

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"
	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// vmFixture builds a KubeVirt VirtualMachine shaped like the ones on a real
// Harvester cluster. Captured from `kubectl get vm windows-server-2022 -o json`,
// which is the most awkward real case: a PVC-backed CD-ROM (an attached ISO), a
// containerDisk-backed CD-ROM (the virtio driver disk), a real root disk and a
// cloud-init volume.
func vmFixture() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kubevirt.io/v1",
		"kind":       "VirtualMachine",
		"metadata":   map[string]interface{}{"name": "windows-server-2022", "namespace": "labs"},
		"spec": map[string]interface{}{
			"runStrategy": "Halted",
			"template": map[string]interface{}{
				"spec": map[string]interface{}{
					"architecture": "amd64",
					"domain": map[string]interface{}{
						"cpu":     map[string]interface{}{"cores": int64(2), "sockets": int64(2), "threads": int64(1)},
						"memory":  map[string]interface{}{"guest": "6Gi"},
						"machine": map[string]interface{}{"type": "q35"},
						"devices": map[string]interface{}{
							"disks": []interface{}{
								map[string]interface{}{"name": "cdrom-disk", "bootOrder": int64(1), "cdrom": map[string]interface{}{"bus": "sata"}},
								map[string]interface{}{"name": "rootdisk", "bootOrder": int64(2), "disk": map[string]interface{}{"bus": "virtio"}},
								map[string]interface{}{"name": "virtio-container-disk", "bootOrder": int64(3), "cdrom": map[string]interface{}{"bus": "sata"}},
								map[string]interface{}{"name": "cloudinitdisk", "disk": map[string]interface{}{"bus": "virtio"}},
							},
							"interfaces": []interface{}{
								map[string]interface{}{"name": "default", "model": "e1000", "macAddress": "7a:86:9b:1a:1f:c4"},
							},
						},
					},
					"volumes": []interface{}{
						map[string]interface{}{"name": "cdrom-disk", "persistentVolumeClaim": map[string]interface{}{"claimName": "iso-pvc"}},
						map[string]interface{}{"name": "rootdisk", "persistentVolumeClaim": map[string]interface{}{"claimName": "root-pvc"}},
						map[string]interface{}{"name": "virtio-container-disk", "containerDisk": map[string]interface{}{"image": "virtio-win"}},
						map[string]interface{}{"name": "cloudinitdisk", "cloudInitNoCloud": map[string]interface{}{"secretRef": map[string]interface{}{"name": "s"}}},
					},
				},
			},
		},
	}}
}

func testPVCs() map[string]PVCInfo {
	return map[string]PVCInfo{
		"labs/iso-pvc":  {capacity: 5 << 30, storageClass: "harvester-longhorn", volumeMode: "Block"},
		"labs/root-pvc": {capacity: 50 << 30, storageClass: "harvester-longhorn", volumeMode: "Block"},
	}
}

// A PVC-backed CD-ROM must NOT be exported as a virtual disk, and a
// containerDisk-backed CD-ROM must not either. Conflating the device type with
// the backing kind silently pulls an attached ISO into the OVA and inflates the
// reported disk total.
func TestHarvesterVMToNode_DeviceVsBackingKind(t *testing.T) {
	node := HarvesterVMToNode(vmFixture(), map[string]bool{}, testPVCs())

	want := map[string]struct {
		device, kind string
		exportable   bool
	}{
		"cdrom-disk":            {"cdrom", "pvc", false},
		"rootdisk":              {"disk", "pvc", true},
		"virtio-container-disk": {"cdrom", "container", false},
		"cloudinitdisk":         {"disk", "cloudinit", false},
	}
	if len(node.Disks) != len(want) {
		t.Fatalf("got %d disks, want %d", len(node.Disks), len(want))
	}
	for _, d := range node.Disks {
		w, ok := want[d.Name]
		if !ok {
			t.Errorf("unexpected disk %q", d.Name)
			continue
		}
		if d.Device != w.device || d.Kind != w.kind {
			t.Errorf("%s: got device=%q kind=%q, want device=%q kind=%q", d.Name, d.Device, d.Kind, w.device, w.kind)
		}
		if got := IsExportableDisk(d); got != w.exportable {
			t.Errorf("%s: IsExportableDisk=%v, want %v", d.Name, got, w.exportable)
		}
	}

	// Only the 50Gi root disk counts; the 5Gi ISO must not inflate the total.
	if node.DiskSizeGB != 50 {
		t.Errorf("DiskSizeGB=%d, want 50 (the 5Gi CD-ROM PVC must be excluded)", node.DiskSizeGB)
	}
}

func TestHarvesterVMToNode_SpecMapping(t *testing.T) {
	node := HarvesterVMToNode(vmFixture(), map[string]bool{}, testPVCs())

	if node.ID != "labs/windows-server-2022" {
		t.Errorf("ID=%q, want namespace-qualified id", node.ID)
	}
	if node.CPU != 4 { // cores(2) * sockets(2) * threads(1)
		t.Errorf("CPU=%d, want 4 (cores*sockets*threads)", node.CPU)
	}
	if node.MemoryMB != 6144 {
		t.Errorf("MemoryMB=%d, want 6144", node.MemoryMB)
	}
	if node.Firmware != "bios" {
		t.Errorf("Firmware=%q, want bios", node.Firmware)
	}
	if node.MachineType != "q35" || node.Architecture != "amd64" {
		t.Errorf("machine=%q arch=%q, want q35/amd64", node.MachineType, node.Architecture)
	}
	if len(node.Networks) != 1 || node.Networks[0].MAC != "7a:86:9b:1a:1f:c4" {
		t.Errorf("networks not mapped: %+v", node.Networks)
	}
}

// The running check is the feature's most important safety property. Harvester's
// PVCs are ReadWriteMany Block volumes, so an export Job CAN mount and read one
// while the VM runs, silently producing a torn image. Nothing in Kubernetes
// prevents that, so this guard must hold.
func TestExportBlockers_RunningVMIsBlocked(t *testing.T) {
	running := map[string]bool{"labs/windows-server-2022": true}
	node := HarvesterVMToNode(vmFixture(), running, testPVCs())

	if node.PowerState != "poweredOn" {
		t.Errorf("PowerState=%q, want poweredOn", node.PowerState)
	}
	if len(node.ExportBlockers) == 0 {
		t.Fatal("a running VM must be blocked from export")
	}
}

// If the VMI list fails we cannot know which VMs are running, so every VM must be
// treated as running. Failing closed blocks an export; failing open corrupts one.
func TestExportBlockers_UnknownRunStateFailsClosed(t *testing.T) {
	node := HarvesterVMToNode(vmFixture(), nil, testPVCs())
	if len(node.ExportBlockers) == 0 {
		t.Fatal("with unknown VMI state the VM must be treated as running and blocked")
	}
}

func TestExportBlockers_StoppedVMIsExportable(t *testing.T) {
	node := HarvesterVMToNode(vmFixture(), map[string]bool{}, testPVCs())
	if len(node.ExportBlockers) != 0 {
		t.Errorf("stopped VM should be exportable, got blockers: %v", node.ExportBlockers)
	}
}

func TestExportBlockers_NonAmd64AndNoDisks(t *testing.T) {
	vm := vmFixture()
	if err := unstructured.SetNestedField(vm.Object, "arm64", "spec", "template", "spec", "architecture"); err != nil {
		t.Fatal(err)
	}
	node := HarvesterVMToNode(vm, map[string]bool{}, testPVCs())
	if !hasBlockerContaining(node.ExportBlockers, "arm64") {
		t.Errorf("arm64 VM must be blocked, got: %v", node.ExportBlockers)
	}

	// A VM whose only volumes are cloud-init has nothing to export.
	vm2 := vmFixture()
	if err := unstructured.SetNestedSlice(vm2.Object, []interface{}{
		map[string]interface{}{"name": "cloudinitdisk", "disk": map[string]interface{}{"bus": "virtio"}},
	}, "spec", "template", "spec", "domain", "devices", "disks"); err != nil {
		t.Fatal(err)
	}
	node2 := HarvesterVMToNode(vm2, map[string]bool{}, testPVCs())
	if !hasBlockerContaining(node2.ExportBlockers, "no PVC-backed disks") {
		t.Errorf("VM with no exportable disks must be blocked, got: %v", node2.ExportBlockers)
	}
}

func hasBlockerContaining(blockers []string, substr string) bool {
	for _, b := range blockers {
		if strings.Contains(b, substr) {
			return true
		}
	}
	return false
}

func TestParseQuantityBytes(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"2Gi", 2 << 30},
		{"1365Mi", 1365 << 20},
		{"512M", 512 * 1000 * 1000},
		{"1073741824", 1073741824},
		{"6Gi", 6 << 30},
		{"garbage", 0},
	}
	for _, c := range cases {
		if got := parseQuantityBytes(c.in); got != c.want {
			t.Errorf("parseQuantityBytes(%q)=%d, want %d", c.in, got, c.want)
		}
	}
}

func inventoryVM(name, ns string) *unstructured.Unstructured {
	vm := vmFixture()
	vm.SetName(name)
	vm.SetNamespace(ns)
	return vm
}

func runningInstance(name, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kubevirt.io/v1", "kind": "VirtualMachineInstance",
		"metadata": map[string]interface{}{"name": name, "namespace": ns},
	}}
}

func claim(ns, name string, gi int64) *corev1.PersistentVolumeClaim {
	block, sc := corev1.PersistentVolumeBlock, "harvester-longhorn"
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeMode: &block, StorageClassName: &sc,
			Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: *resource.NewQuantity(gi<<30, resource.BinarySI)}},
		},
	}
}

func TestHandleGetHarvesterInventoryBuildsTheTree(t *testing.T) {
	// The API server's list order is not something the handler may rely on, so the
	// fake returns the VMs in a deliberately scrambled order (no rotation of it is
	// sorted): the tree can only come out sorted if the handler sorts it.
	scrambled := []struct{ ns, name string }{
		{"zeta", "stopped-vm"}, {"alpha", "stopped-vm"}, {"mid", "stopped-vm"},
		{"dev", "stopped-vm"}, {"dev", "busy-vm"}, {"labs", "stopped-vm"}, {"beta", "stopped-vm"},
	}
	items := make([]unstructured.Unstructured, 0, len(scrambled))
	var claims []runtime.Object
	seen := map[string]bool{}
	for _, v := range scrambled {
		items = append(items, *inventoryVM(v.name, v.ns))
		if !seen[v.ns] {
			seen[v.ns] = true
			claims = append(claims, claim(v.ns, "root-pvc", 50), claim(v.ns, "iso-pvc", 5))
		}
	}

	clients := testutil.NewClientsWithListKinds(map[schema.GroupVersionResource]string{
		kube.VMGVR: "VirtualMachineList", kube.VMIKubevirtGVR: "VirtualMachineInstanceList",
	})
	clients.Clientset = fake.NewSimpleClientset(claims...)
	clients.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("list", "virtualmachines",
		func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, &unstructured.UnstructuredList{
				Object: map[string]interface{}{"apiVersion": "kubevirt.io/v1", "kind": "VirtualMachineList"},
				Items:  items,
			}, nil
		})
	if _, err := clients.Dynamic.Resource(kube.VMIKubevirtGVR).Namespace("dev").Create(t.Context(), runningInstance("busy-vm", "dev"), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	rr := testutil.Do(HandleGetHarvesterInventory(clients), "GET", "/x", nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body)
	}
	var root Node
	if err := json.Unmarshal(rr.Body.Bytes(), &root); err != nil {
		t.Fatal(err)
	}
	if root.ID != "harvester" || root.Type != "datacenter" {
		t.Fatalf("root = %s/%s", root.ID, root.Type)
	}
	got := make([]string, 0, len(root.Children))
	var dev Node
	for _, ns := range root.Children {
		got = append(got, ns.Name)
		if ns.Name == "dev" {
			dev = ns
		}
	}
	if want := "alpha,beta,dev,labs,mid,zeta"; strings.Join(got, ",") != want {
		t.Errorf("namespaces = %v, want sorted %s", got, want)
	}

	// VMs inside a namespace are sorted too, and a running VM must not be offered for
	// export while a stopped one may be.
	if len(dev.Children) != 2 || dev.Children[0].Name != "busy-vm" || dev.Children[1].Name != "stopped-vm" {
		t.Fatalf("VMs in dev = %+v", dev.Children)
	}
	busy, stopped := dev.Children[0], dev.Children[1]
	if !strings.Contains(strings.Join(busy.ExportBlockers, " "), "running") {
		t.Errorf("a running VM must carry an export blocker: %v", busy.ExportBlockers)
	}
	if len(stopped.ExportBlockers) != 0 {
		t.Errorf("a stopped VM with PVC-backed disks is exportable: %v", stopped.ExportBlockers)
	}
}

func TestHandleGetHarvesterInventoryFailures(t *testing.T) {
	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "virtualmachines"}, "x", errors.New("no"))
	kinds := map[schema.GroupVersionResource]string{kube.VMGVR: "VirtualMachineList", kube.VMIKubevirtGVR: "VirtualMachineInstanceList"}

	// A caller who may not list VMs gets the API server's 403; any other failure stays 500.
	for name, tc := range map[string]struct {
		err  error
		want int
	}{"forbidden is 403": {forbidden, http.StatusForbidden}, "a non-API error is 500": {errors.New("connection reset"), http.StatusInternalServerError}} {
		t.Run(name, func(t *testing.T) {
			clients := testutil.NewClientsWithListKinds(kinds)
			testutil.Fail(clients, "list", "virtualmachines", tc.err)
			if rr := testutil.Do(HandleGetHarvesterInventory(clients), "GET", "/x", nil, nil); rr.Code != tc.want {
				t.Errorf("status %d, want %d: %s", rr.Code, tc.want, rr.Body)
			}
		})
	}

	t.Run("an empty cluster is an empty tree, not an error", func(t *testing.T) {
		rr := testutil.Do(HandleGetHarvesterInventory(testutil.NewClientsWithListKinds(kinds)), "GET", "/x", nil, nil)
		var root Node
		if rr.Code != http.StatusOK || json.Unmarshal(rr.Body.Bytes(), &root) != nil || len(root.Children) != 0 {
			t.Errorf("status %d body %s", rr.Code, rr.Body)
		}
	})
}
