// harvester.go
//
// Cluster-wide Harvester/KubeVirt VM inventory, shaped as the same InventoryNode
// tree the vCenter explorer already renders (Cluster -> Namespace -> VirtualMachine),
// so the existing React tree components can be reused unchanged.
//
// This feeds the VM Export page. See docs/architecture-notes.md.
package inventory

import (
	"context"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/httpx"

	log "github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// PVCInfo is the subset of a PersistentVolumeClaim the export flow cares about.
type PVCInfo struct {
	capacity     int64
	storageClass string
	volumeMode   string
}

// HandleGetHarvesterInventory returns the whole cluster's KubeVirt VMs as an
// Node tree. It is read-only and best-effort: a namespace that fails to
// list is logged and skipped rather than failing the whole request.
func HandleGetHarvesterInventory(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		vms, err := clients.Dynamic.Resource(kube.VMGVR).Namespace("").List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Errorf("Failed to list VirtualMachines: %v", err)
			httpx.RespondWithAPIErrorMsg(w, err, "Failed to list VirtualMachines: "+err.Error())
			return
		}

		var warnings []string
		running, err := runningVMNames(ctx, clients)
		if err != nil {
			warnings = append(warnings, "Could not list VirtualMachineInstances ("+briefCause(err)+"): every VM is shown as running, so none can be exported.")
		}
		pvcs, err := PVCIndex(ctx, clients)
		if err != nil {
			warnings = append(warnings, "Could not list PersistentVolumeClaims ("+briefCause(err)+"): disk sizes and storage classes are unknown.")
		}

		byNamespace := map[string][]Node{}
		for i := range vms.Items {
			vm := &vms.Items[i]
			node := HarvesterVMToNode(vm, running, pvcs)
			byNamespace[vm.GetNamespace()] = append(byNamespace[vm.GetNamespace()], node)
		}

		namespaces := make([]string, 0, len(byNamespace))
		for ns := range byNamespace {
			namespaces = append(namespaces, ns)
		}
		sort.Strings(namespaces)

		root := Node{ID: "harvester", Name: "Harvester Cluster", Type: "datacenter", Warnings: warnings}
		for _, ns := range namespaces {
			children := byNamespace[ns]
			sort.Slice(children, func(a, b int) bool { return children[a].Name < children[b].Name })
			root.Children = append(root.Children, Node{
				ID:       "ns/" + ns,
				Name:     ns,
				Type:     "namespace",
				Children: children,
			})
		}

		log.Debugf("Built Harvester inventory: %d namespaces, %d VMs", len(namespaces), len(vms.Items))
		httpx.RespondWithJSON(w, http.StatusOK, root)
	}
}

// runningVMNames returns the set of "namespace/name" that currently have a VMI.
// A VMI existing is the authoritative signal that a VM's volumes are in use and
// therefore MUST NOT be read for export. When the VMIs cannot be listed it returns a
// nil set and the error: every VM is then treated as running, which is the safe
// direction (it blocks export rather than corrupting an image).
func runningVMNames(ctx context.Context, clients *kube.Clients) (map[string]bool, error) {
	out := map[string]bool{}
	list, err := clients.Dynamic.Resource(kube.VMIKubevirtGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Warnf("Failed to list VirtualMachineInstances, treating all VMs as running: %v", err)
		return nil, err
	}
	for _, vmi := range list.Items {
		out[vmi.GetNamespace()+"/"+vmi.GetName()] = true
	}
	return out, nil
}

// PVCIndex maps "namespace/name" to the PVC details the export needs. A failed list is
// returned, not hidden: "no claims" and "could not list claims" call for different things
// (the second is usually the caller's permissions), and the index is empty either way.
func PVCIndex(ctx context.Context, clients *kube.Clients) (map[string]PVCInfo, error) {
	out := map[string]PVCInfo{}
	list, err := clients.Clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Warnf("Failed to list PersistentVolumeClaims, disk sizes will be unknown: %v", err)
		return out, err
	}
	for i := range list.Items {
		p := &list.Items[i]
		info := PVCInfo{}
		if q, ok := p.Spec.Resources.Requests["storage"]; ok {
			info.capacity = q.Value()
		}
		if p.Spec.StorageClassName != nil {
			info.storageClass = *p.Spec.StorageClassName
		}
		if p.Spec.VolumeMode != nil {
			info.volumeMode = string(*p.Spec.VolumeMode)
		}
		out[p.Namespace+"/"+p.Name] = info
	}
	return out, nil
}

// HarvesterVMToNode flattens one KubeVirt VirtualMachine into an Node.
func HarvesterVMToNode(vm *unstructured.Unstructured, running map[string]bool, pvcs map[string]PVCInfo) Node {
	ns, name := vm.GetNamespace(), vm.GetName()
	key := ns + "/" + name

	node := Node{
		ID:        key,
		Name:      name,
		Type:      "VirtualMachine",
		Namespace: ns,
		Folder:    ns,
	}

	// running == nil means the VMI list failed; assume running (safe direction).
	isRunning := running == nil || running[key]
	if isRunning {
		node.PowerState = "poweredOn"
	} else {
		node.PowerState = "poweredOff"
	}

	node.RunStrategy, _, _ = unstructured.NestedString(vm.Object, "spec", "runStrategy")

	spec, _, _ := unstructured.NestedMap(vm.Object, "spec", "template", "spec")
	if spec == nil {
		node.ExportBlockers = append(node.ExportBlockers, "VM has no template spec")
		return node
	}
	node.Architecture, _, _ = unstructured.NestedString(spec, "architecture")

	domain, _, _ := unstructured.NestedMap(spec, "domain")
	if domain != nil {
		node.CPU = vcpuCount(domain)
		node.MemoryMB = memoryMB(domain)
		node.MachineType, _, _ = unstructured.NestedString(domain, "machine", "type")
		node.Firmware = firmwareKind(domain)
		node.Disks = harvesterDisks(domain, spec, ns, pvcs)
		node.Networks = harvesterNetworks(domain)
	}

	var totalBytes int64
	for _, d := range node.Disks {
		if IsExportableDisk(d) {
			totalBytes += d.Capacity
		}
	}
	node.DiskSizeGB = totalBytes / (1024 * 1024 * 1024)

	node.ExportBlockers = ExportBlockers(&node, isRunning)
	return node
}

// vcpuCount is cores*sockets*threads, defaulting each to 1, falling back to the
// CPU resource request when domain.cpu is absent.
func vcpuCount(domain map[string]interface{}) int32 {
	cpu, _, _ := unstructured.NestedMap(domain, "cpu")
	if cpu == nil {
		return 0
	}
	get := func(k string) int64 {
		v, found, err := unstructured.NestedInt64(cpu, k)
		if !found || err != nil || v <= 0 {
			return 1
		}
		return v
	}
	return clampInt32(get("cores") * get("sockets") * get("threads"))
}

// memoryMB prefers domain.memory.guest and falls back to the memory request.
func memoryMB(domain map[string]interface{}) int32 {
	if s, found, _ := unstructured.NestedString(domain, "memory", "guest"); found && s != "" {
		return clampInt32(parseQuantityBytes(s) / (1024 * 1024))
	}
	if s, found, _ := unstructured.NestedString(domain, "resources", "requests", "memory"); found && s != "" {
		return clampInt32(parseQuantityBytes(s) / (1024 * 1024))
	}
	return 0
}

// firmwareKind reports "efi" when an EFI bootloader is configured, else "bios".
func firmwareKind(domain map[string]interface{}) string {
	if _, found, _ := unstructured.NestedMap(domain, "firmware", "bootloader", "efi"); found {
		return "efi"
	}
	return "bios"
}

// harvesterDisks correlates domain.devices.disks with spec.volumes so each disk
// carries its backing kind and, for PVCs, its real capacity.
func harvesterDisks(domain, spec map[string]interface{}, ns string, pvcs map[string]PVCInfo) []Disk {
	disks, _, _ := unstructured.NestedSlice(domain, "devices", "disks")
	volumes, _, _ := unstructured.NestedSlice(spec, "volumes")

	// volume name -> backing kind and PVC claim name
	kindByVolume := map[string]string{}
	claimByVolume := map[string]string{}
	for _, raw := range volumes {
		v, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		vname, _, _ := unstructured.NestedString(v, "name")
		switch {
		case hasKey(v, "persistentVolumeClaim"):
			kindByVolume[vname] = "pvc"
			claimByVolume[vname], _, _ = unstructured.NestedString(v, "persistentVolumeClaim", "claimName")
		case hasKey(v, "dataVolume"):
			kindByVolume[vname] = "pvc"
			claimByVolume[vname], _, _ = unstructured.NestedString(v, "dataVolume", "name")
		case hasKey(v, "cloudInitNoCloud"), hasKey(v, "cloudInitConfigDrive"):
			kindByVolume[vname] = "cloudinit"
		case hasKey(v, "containerDisk"):
			kindByVolume[vname] = "container"
		default:
			kindByVolume[vname] = "other"
		}
	}

	out := make([]Disk, 0, len(disks))
	for i, raw := range disks {
		d, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _, _ := unstructured.NestedString(d, "name")
		// Device type (disk/cdrom/lun) and backing kind (pvc/cloudinit/...) are
		// independent: a CD-ROM can be PVC-backed (an attached ISO) or
		// containerDisk-backed (e.g. the virtio driver disk on Windows guests).
		disk := Disk{Name: name, UnitNum: int32(i), Kind: kindByVolume[name], Device: "disk"}

		if bus, found, _ := unstructured.NestedString(d, "disk", "bus"); found {
			disk.BusType = bus
		} else if bus, found, _ := unstructured.NestedString(d, "cdrom", "bus"); found {
			disk.BusType = bus
			disk.Device = "cdrom"
		} else if bus, found, _ := unstructured.NestedString(d, "lun", "bus"); found {
			disk.BusType = bus
			disk.Device = "lun"
		}
		if bo, found, _ := unstructured.NestedInt64(d, "bootOrder"); found {
			disk.BootOrder = clampInt32(bo)
		}
		if claim := claimByVolume[name]; claim != "" {
			disk.PVCName = claim
			if info, ok := pvcs[ns+"/"+claim]; ok {
				disk.Capacity = info.capacity
				disk.StorageClass = info.storageClass
				disk.VolumeMode = info.volumeMode
			}
		}
		out = append(out, disk)
	}
	return out
}

// harvesterNetworks flattens domain.devices.interfaces.
func harvesterNetworks(domain map[string]interface{}) []Network {
	ifaces, _, _ := unstructured.NestedSlice(domain, "devices", "interfaces")
	out := make([]Network, 0, len(ifaces))
	for i, raw := range ifaces {
		n, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _, _ := unstructured.NestedString(n, "name")
		mac, _, _ := unstructured.NestedString(n, "macAddress")
		model, _, _ := unstructured.NestedString(n, "model")
		out = append(out, Network{Name: name, ID: model, MAC: mac, Key: int32(i)})
	}
	return out
}

// ExportBlockers lists the reasons this VM cannot be exported right now. An empty
// result means the VM is exportable.
//
// The running check is the important one: Harvester's PVCs are ReadWriteMany Block
// volumes, so a Job CAN mount and read one while the VM runs — producing a torn,
// inconsistent image with no error. Nothing in Kubernetes prevents this, so it is
// enforced here and re-checked before the export Job starts.
func ExportBlockers(node *Node, isRunning bool) []string {
	var blockers []string
	if isRunning {
		blockers = append(blockers, "VM is running: power it off, or enable the snapshot option to export a crash-consistent copy")
	}
	if node.Architecture != "" && node.Architecture != "amd64" {
		blockers = append(blockers, "unsupported architecture "+node.Architecture+": OVF export supports amd64 only")
	}
	var exportable int
	for _, d := range node.Disks {
		if IsExportableDisk(d) {
			exportable++
		}
	}
	if exportable == 0 {
		blockers = append(blockers, "VM has no PVC-backed disks to export")
	}
	return blockers
}

// IsExportableDisk reports whether a disk contributes a virtual disk to the OVA.
// Only PVC-backed disks qualify: CD-ROMs become empty drives in the descriptor,
// containerDisks are image layers rather than VM state, and cloud-init volumes are
// deliberately excluded because they carry credentials.
func IsExportableDisk(d Disk) bool {
	return d.Kind == "pvc" && d.Device == "disk"
}

func hasKey(m map[string]interface{}, k string) bool {
	_, ok := m[k]
	return ok
}

// parseQuantityBytes parses the Kubernetes quantity subset KubeVirt emits for
// memory (e.g. "2Gi", "1365Mi", "512M", "1073741824").
func parseQuantityBytes(s string) int64 {
	s = strings.TrimSpace(s)
	suffixes := []struct {
		suffix string
		mult   int64
	}{
		{"Ki", 1 << 10}, {"Mi", 1 << 20}, {"Gi", 1 << 30}, {"Ti", 1 << 40},
		{"K", 1000}, {"M", 1000 * 1000}, {"G", 1000 * 1000 * 1000}, {"T", 1000 * 1000 * 1000 * 1000},
	}
	for _, x := range suffixes {
		if strings.HasSuffix(s, x.suffix) {
			n, err := strconv.ParseFloat(strings.TrimSuffix(s, x.suffix), 64)
			if err != nil {
				log.Warnf("Could not parse quantity %q: %v", s, err)
				return 0
			}
			return int64(n * float64(x.mult))
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		log.Warnf("Could not parse quantity %q: %v", s, err)
		return 0
	}
	return n
}

// clampInt32 narrows an int64 without wrapping around on absurd values.
func clampInt32(v int64) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v) //nolint:gosec // range checked above
}

// briefCause is a short reason for a warning: "forbidden" for a permission problem,
// otherwise the error text.
func briefCause(err error) string {
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		return "forbidden: no permission"
	}
	return err.Error()
}
