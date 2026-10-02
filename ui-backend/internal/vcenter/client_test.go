// client_test.go
package vcenter

import (
	"context"
	"testing"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/inventory"
	"github.com/vmware/govmomi/simulator"
)

// withSimulator runs f against an in-process vCenter (govmomi's vcsim) with its
// default model: one datacenter "DC0" with hosts, datastores and VMs.
func withSimulator(t *testing.T, f func(ctx context.Context, creds Credentials)) {
	t.Helper()
	model := simulator.VPX()
	defer model.Remove()
	if err := model.Create(); err != nil {
		t.Fatal(err)
	}
	s := model.Service.NewServer()
	defer s.Close()
	pw, _ := s.URL.User.Password()
	f(context.Background(), Credentials{
		URL:        s.URL.Scheme + "://" + s.URL.Host + s.URL.Path,
		Username:   s.URL.User.Username(),
		Password:   pw,
		Datacenter: "DC0",
	})
}

func walk(n *inventory.Node, visit func(*inventory.Node)) {
	visit(n)
	for i := range n.Children {
		walk(&n.Children[i], visit)
	}
}

func findVM(root *inventory.Node, name string) *inventory.Node {
	var hit *inventory.Node
	walk(root, func(n *inventory.Node) {
		if n.Type == "VirtualMachine" && n.Name == name {
			hit = n
		}
	})
	return hit
}

func mustInventory(t *testing.T, ctx context.Context, creds Credentials) *inventory.Node {
	t.Helper()
	root, err := GetInventory(ctx, creds)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// The tree shape is what the frontend renders: datacenter -> VMs with their
// NICs, disks and sizing. These values are the simulator's defaults.
func TestGetInventoryTree(t *testing.T) {
	withSimulator(t, func(ctx context.Context, creds Credentials) {
		root := mustInventory(t, ctx, creds)
		if root.Name != "DC0" || root.Type != "datacenter" {
			t.Fatalf("root = %q/%q, want DC0/datacenter", root.Name, root.Type)
		}
		var vms []*inventory.Node
		walk(root, func(n *inventory.Node) {
			if n.Type == "VirtualMachine" {
				vms = append(vms, n)
			}
		})
		if len(vms) != 4 {
			t.Fatalf("got %d VMs, want 4", len(vms))
		}
		for _, vm := range vms {
			if vm.PowerState != "poweredOn" || vm.CPU != 1 || vm.MemoryMB != 32 || len(vm.Networks) != 1 || len(vm.Disks) != 1 {
				t.Errorf("VM %s: power=%q cpu=%d mem=%d nets=%d disks=%d", vm.Name, vm.PowerState, vm.CPU, vm.MemoryMB, len(vm.Networks), len(vm.Disks))
			}
			if vm.Networks[0].MAC == "" || vm.Networks[0].Key == 0 {
				t.Errorf("VM %s: NIC lacks MAC/key (needed for MAC edits): %+v", vm.Name, vm.Networks[0])
			}
		}
	})
}

func TestGetInventoryAutoDiscover(t *testing.T) {
	withSimulator(t, func(ctx context.Context, creds Credentials) {
		creds.Datacenter = "" // auto-discover must find DC0 on its own
		root, err := GetInventoryAutoDiscover(ctx, creds)
		if err != nil {
			t.Fatal(err)
		}
		if findVM(root, "DC0_H0_VM0") == nil {
			t.Errorf("auto-discovered tree lacks DC0_H0_VM0")
		}
	})
}

func TestGetInventoryRejectsUnknownDatacenter(t *testing.T) {
	withSimulator(t, func(ctx context.Context, creds Credentials) {
		// (vcsim accepts any password, so bad credentials cannot be tested here.)
		bad := creds
		bad.Datacenter = "does-not-exist"
		if _, err := GetInventory(ctx, bad); err == nil {
			t.Error("unknown datacenter must fail")
		}
	})
}

func TestPowerOpVM(t *testing.T) {
	withSimulator(t, func(ctx context.Context, creds Credentials) {
		const vm = "DC0_H0_VM0"
		power := func() string { return findVM(mustInventory(t, ctx, creds), vm).PowerState }

		if err := PowerOpVM(ctx, creds, vm, "off"); err != nil {
			t.Fatal(err)
		}
		if got := power(); got != "poweredOff" {
			t.Errorf("after off: %q", got)
		}
		if err := PowerOpVM(ctx, creds, vm, "on"); err != nil {
			t.Fatal(err)
		}
		if got := power(); got != "poweredOn" {
			t.Errorf("after on: %q", got)
		}
		if err := PowerOpVM(ctx, creds, vm, "reset"); err != nil {
			t.Fatal(err)
		}
		// No guest tools in the simulator: shutdown falls back to power off.
		if err := PowerOpVM(ctx, creds, vm, "shutdown"); err != nil {
			t.Fatal(err)
		}
		if got := power(); got != "poweredOff" {
			t.Errorf("after shutdown fallback: %q", got)
		}
		if err := PowerOpVM(ctx, creds, vm, "explode"); err == nil {
			t.Error("unsupported operation must fail")
		}
		if err := PowerOpVM(ctx, creds, "no-such-vm", "on"); err == nil {
			t.Error("unknown VM must fail")
		}
	})
}

func TestRenameVM(t *testing.T) {
	withSimulator(t, func(ctx context.Context, creds Credentials) {
		if err := RenameVM(ctx, creds, "DC0_H0_VM1", "renamed-vm"); err != nil {
			t.Fatal(err)
		}
		root := mustInventory(t, ctx, creds)
		if findVM(root, "renamed-vm") == nil || findVM(root, "DC0_H0_VM1") != nil {
			t.Errorf("rename not reflected in the inventory")
		}
		if err := RenameVM(ctx, creds, "no-such-vm", "x"); err == nil {
			t.Error("renaming an unknown VM must fail")
		}
	})
}

func TestUpdateVMNetworkMAC(t *testing.T) {
	withSimulator(t, func(ctx context.Context, creds Credentials) {
		const vm = "DC0_C0_RP0_VM0"
		nic := findVM(mustInventory(t, ctx, creds), vm).Networks[0]
		const mac = "00:50:56:aa:bb:cc"
		if err := UpdateVMNetworkMAC(ctx, creds, vm, nic.Key, mac); err != nil {
			t.Fatal(err)
		}
		if got := findVM(mustInventory(t, ctx, creds), vm).Networks[0].MAC; got != mac {
			t.Errorf("MAC = %q, want %q", got, mac)
		}
		if err := UpdateVMNetworkMAC(ctx, creds, vm, 99999, mac); err == nil {
			t.Error("unknown device key must fail")
		}
	})
}
