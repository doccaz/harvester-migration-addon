// client.go
package vcenter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/httpx"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/inventory"

	log "github.com/sirupsen/logrus"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

// GetInventory connects to vCenter and returns the inventory tree.
func GetInventory(ctx context.Context, creds Credentials) (*inventory.Node, error) {
	fullURL := creds.URL
	if !strings.HasPrefix(fullURL, "https://") && !strings.HasPrefix(fullURL, "http://") {
		fullURL = "https://" + fullURL
	}

	u, err := url.Parse(fullURL)
	if err != nil {
		return nil, err
	}
	u.User = url.UserPassword(creds.Username, creds.Password)

	log.Infof("Connecting to vCenter at %s", creds.URL)
	c, err := govmomi.NewClient(ctx, u, true)
	if err != nil {
		return nil, err
	}
	defer logout(ctx, c)

	finder := find.NewFinder(c.Client, true)
	dc, err := finder.Datacenter(ctx, creds.Datacenter)
	if err != nil {
		return nil, err
	}
	finder.SetDatacenter(dc)

	rootNode := &inventory.Node{
		Name: dc.Name(),
		Type: "datacenter",
	}

	folders, err := dc.Folders(ctx)
	if err != nil {
		return nil, err
	}

	rootFolder := object.NewFolder(c.Client, folders.VmFolder.Reference())
	children, err := rootFolder.Children(ctx)
	if err != nil {
		return nil, err
	}

	for _, child := range children {
		// Initialize recursion with an empty string
		node, err := processEntity(ctx, c, child, "")
		if err != nil {
			log.Warnf("Could not process entity %s: %v", child.Reference().Value, err)
			continue
		}
		if node != nil {
			rootNode.Children = append(rootNode.Children, *node)
		}
	}

	log.Debugf("Constructed vCenter inventory tree: %+v", rootNode)
	return rootNode, nil
}

// processEntity recursively processes vCenter inventory objects.
func processEntity(ctx context.Context, c *govmomi.Client, entity object.Reference, folderPath string) (*inventory.Node, error) {
	ref := entity.Reference()

	var me mo.ManagedEntity
	pc := property.DefaultCollector(c.Client)
	if err := pc.RetrieveOne(ctx, ref, []string{"name"}, &me); err != nil {
		return nil, err
	}

	node := &inventory.Node{
		ID:   ref.Value,
		Name: me.Name,
		Type: ref.Type,
	}

	switch e := entity.(type) {
	case *object.VirtualMachine:
		var mvm mo.VirtualMachine
		err := pc.RetrieveOne(ctx, ref, []string{"guest", "summary", "config", "network", "config.hardware.device", "runtime", "datastore"}, &mvm)
		if err != nil {
			return nil, err
		}

		log.Debugf("Raw VM data from vCenter for %s: %+v", me.Name, mvm)

		var vmNetworks []inventory.Network
		var vmDisks []inventory.Disk

		if mvm.Config != nil {
			// Get the list of virtual devices from the managed object
			deviceList := object.VirtualDeviceList(mvm.Config.Hardware.Device)

			// Map to find controller types
			controllers := make(map[int32]string)
			for _, device := range deviceList {
				d := device.GetVirtualDevice()
				switch device.(type) {
				case *types.VirtualLsiLogicController, *types.VirtualLsiLogicSASController, *types.VirtualBusLogicController, *types.ParaVirtualSCSIController:
					controllers[d.Key] = "scsi"
				case *types.VirtualIDEController:
					controllers[d.Key] = "ide"
				case *types.VirtualSATAController:
					controllers[d.Key] = "sata"
				case *types.VirtualNVMEController:
					controllers[d.Key] = "nvme"
				}
			}

			// Find all network card devices and disks
			for _, device := range deviceList {
				// Use a type assertion to see if the device is a network card
				if card, ok := device.(types.BaseVirtualEthernetCard); ok {
					// Get the backing info from the network card
					backing := card.GetVirtualEthernetCard().Backing

					netName := "unknown"
					netID := ""
					switch backingInfo := backing.(type) {
					case *types.VirtualEthernetCardNetworkBackingInfo:
						netName = backingInfo.DeviceName
						if backingInfo.Network != nil {
							netID = backingInfo.Network.Value
						}
					case *types.VirtualEthernetCardDistributedVirtualPortBackingInfo:
						netID = backingInfo.Port.PortgroupKey
						netName = backingInfo.Port.PortgroupKey // fallback: raw moref key
						pgRef := types.ManagedObjectReference{
							Type:  "DistributedVirtualPortgroup",
							Value: backingInfo.Port.PortgroupKey,
						}
						var dvpg mo.DistributedVirtualPortgroup
						if err := pc.RetrieveOne(ctx, pgRef, []string{"name", "config.distributedVirtualSwitch"}, &dvpg); err == nil {
							pgName := dvpg.Name
							if dvpg.Config.DistributedVirtualSwitch != nil {
								var dvsMe mo.ManagedEntity
								if err2 := pc.RetrieveOne(ctx, *dvpg.Config.DistributedVirtualSwitch, []string{"name"}, &dvsMe); err2 == nil {
									netName = dvsMe.Name + "/" + pgName
								} else {
									log.Warnf("Could not resolve dvSwitch name for portgroup %s: %v", backingInfo.Port.PortgroupKey, err2)
									netName = pgName
								}
							} else {
								netName = pgName
							}
						} else {
							log.Warnf("Could not resolve DVPortgroup %s: %v", backingInfo.Port.PortgroupKey, err)
						}
					}

					vmNetworks = append(vmNetworks, inventory.Network{
						Name: netName,
						ID:   netID,
						MAC:  card.GetVirtualEthernetCard().MacAddress,
						Key:  card.GetVirtualEthernetCard().Key,
					})
				} else if disk, ok := device.(*types.VirtualDisk); ok {
					busType := "unknown"
					if t, ok := controllers[disk.ControllerKey]; ok {
						busType = t
					}
					name := "Disk"
					if disk.DeviceInfo != nil {
						name = disk.DeviceInfo.GetDescription().Label
					}
					vmDisks = append(vmDisks, inventory.Disk{
						Name:     name,
						Capacity: disk.CapacityInBytes,
						BusType:  busType,
						UnitNum:  *disk.UnitNumber,
					})
				}
			}
		} else {
			log.Warnf("VM '%s' has nil Config, skipping device processing", me.Name)
		}

		log.Debugf("Successfully found networks for VM '%s': %v\n", me.Name, vmNetworks)

		node.Networks = vmNetworks
		node.Disks = vmDisks
		node.DiskSizeGB = mvm.Summary.Storage.Committed / (1024 * 1024 * 1024)

		node.CPU = mvm.Summary.Config.NumCpu
		node.MemoryMB = mvm.Summary.Config.MemorySizeMB

		node.PowerState = string(mvm.Runtime.PowerState)
		node.Folder = folderPath // Store the accumulated folder path

		// Auto-detect datastore ID from the VM's datastore references
		if len(mvm.Datastore) > 0 {
			node.DatastoreID = mvm.Datastore[0].Value
			// Try to get the datastore name
			var dsmo mo.Datastore
			if dsErr := pc.RetrieveOne(ctx, mvm.Datastore[0], []string{"name"}, &dsmo); dsErr == nil {
				node.DatastoreName = dsmo.Name
			}
		}

		return node, nil

	case *object.Folder:
		// Build the path: parent/current
		childPath := folderPath
		if childPath != "" {
			childPath += "/"
		}
		childPath += me.Name

		children, err := e.Children(ctx)
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			childNode, err := processEntity(ctx, c, child, childPath)
			if err != nil {
				log.Warnf("Could not process child entity %s: %v", child.Reference().Value, err)
				continue
			}
			if childNode != nil {
				node.Children = append(node.Children, *childNode)
			}
		}
		return node, nil

	case *object.ClusterComputeResource:
		rp, err := e.ResourcePool(ctx)
		if err != nil {
			return node, nil
		}
		var mrp mo.ResourcePool
		err = pc.RetrieveOne(ctx, rp.Reference(), []string{"vm"}, &mrp)
		if err != nil {
			return nil, err
		}
		for _, vmRef := range mrp.Vm {
			// Pass the existing folderPath through clusters
			childNode, err := processEntity(ctx, c, object.NewVirtualMachine(c.Client, vmRef), folderPath)
			if err != nil {
				log.Warnf("Could not process child vm in cluster %s: %v", vmRef.Value, err)
				continue
			}
			if childNode != nil {
				node.Children = append(node.Children, *childNode)
			}
		}
		return node, nil

	default:
		return nil, nil
	}
}

// PowerOpVM performs a power operation on a VM.
func PowerOpVM(ctx context.Context, creds Credentials, vmName string, op string) error {
	fullURL := creds.URL
	if !strings.HasPrefix(fullURL, "https://") && !strings.HasPrefix(fullURL, "http://") {
		fullURL = "https://" + fullURL
	}
	u, err := url.Parse(fullURL)
	if err != nil {
		return err
	}
	u.User = url.UserPassword(creds.Username, creds.Password)

	c, err := govmomi.NewClient(ctx, u, true)
	if err != nil {
		return err
	}
	defer logout(ctx, c)

	finder := find.NewFinder(c.Client, true)
	dc, err := finder.Datacenter(ctx, creds.Datacenter)
	if err != nil {
		return err
	}
	finder.SetDatacenter(dc)

	vm, err := finder.VirtualMachine(ctx, vmName)
	if err != nil {
		return err
	}

	var task *object.Task
	switch op {
	case "on":
		task, err = vm.PowerOn(ctx)
	case "off":
		task, err = vm.PowerOff(ctx)
	case "reset":
		task, err = vm.Reset(ctx)
	case "shutdown":
		err = vm.ShutdownGuest(ctx)
		if err != nil {
			// Fallback to power off if shutdown fails (e.g. tools not installed)
			log.Warnf("Guest shutdown failed for %s, falling back to power off: %v", vmName, err)
			task, err = vm.PowerOff(ctx)
		} else {
			return nil // ShutdownGuest doesn't return a task, it's just an error if it fails to initiate
		}
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedOperation, op)
	}

	if err != nil {
		return err
	}

	if task != nil {
		return task.Wait(ctx)
	}
	return nil
}

// RenameVM renames a VM in vCenter.
func RenameVM(ctx context.Context, creds Credentials, oldName string, newName string) error {
	fullURL := creds.URL
	if !strings.HasPrefix(fullURL, "https://") && !strings.HasPrefix(fullURL, "http://") {
		fullURL = "https://" + fullURL
	}
	u, err := url.Parse(fullURL)
	if err != nil {
		return err
	}
	u.User = url.UserPassword(creds.Username, creds.Password)

	c, err := govmomi.NewClient(ctx, u, true)
	if err != nil {
		return err
	}
	defer logout(ctx, c)

	finder := find.NewFinder(c.Client, true)
	dc, err := finder.Datacenter(ctx, creds.Datacenter)
	if err != nil {
		return err
	}
	finder.SetDatacenter(dc)

	vm, err := finder.VirtualMachine(ctx, oldName)
	if err != nil {
		return err
	}

	task, err := vm.Rename(ctx, newName)
	if err != nil {
		return err
	}

	return task.Wait(ctx)
}

// UpdateVMNetworkMAC updates the MAC address of a specific network device.
func UpdateVMNetworkMAC(ctx context.Context, creds Credentials, vmName string, deviceKey int32, newMAC string) error {
	fullURL := creds.URL
	if !strings.HasPrefix(fullURL, "https://") && !strings.HasPrefix(fullURL, "http://") {
		fullURL = "https://" + fullURL
	}
	u, err := url.Parse(fullURL)
	if err != nil {
		return err
	}
	u.User = url.UserPassword(creds.Username, creds.Password)

	c, err := govmomi.NewClient(ctx, u, true)
	if err != nil {
		return err
	}
	defer logout(ctx, c)

	finder := find.NewFinder(c.Client, true)
	dc, err := finder.Datacenter(ctx, creds.Datacenter)
	if err != nil {
		return err
	}
	finder.SetDatacenter(dc)

	vm, err := finder.VirtualMachine(ctx, vmName)
	if err != nil {
		return err
	}

	var mvm mo.VirtualMachine
	pc := property.DefaultCollector(c.Client)
	if err := pc.RetrieveOne(ctx, vm.Reference(), []string{"config.hardware.device"}, &mvm); err != nil {
		return err
	}

	deviceList := object.VirtualDeviceList(mvm.Config.Hardware.Device)
	device := deviceList.FindByKey(deviceKey)
	if device == nil {
		return &DeviceNotFoundError{Key: deviceKey}
	}

	nic, ok := device.(types.BaseVirtualEthernetCard)
	if !ok {
		return fmt.Errorf("device with key %d is not a network card", deviceKey)
	}

	card := nic.GetVirtualEthernetCard()
	card.MacAddress = newMAC
	card.AddressType = "manual"

	spec := types.VirtualMachineConfigSpec{
		DeviceChange: []types.BaseVirtualDeviceConfigSpec{
			&types.VirtualDeviceConfigSpec{
				Operation: types.VirtualDeviceConfigSpecOperationEdit,
				Device:    device,
			},
		},
	}

	task, err := vm.Reconfigure(ctx, spec)
	if err != nil {
		return err
	}

	return task.Wait(ctx)
}

// GetInventoryAutoDiscover connects to vCenter and auto-discovers the first datacenter.
// This is used by Forklift, which doesn't store the datacenter name in the Provider spec.
func GetInventoryAutoDiscover(ctx context.Context, creds Credentials) (*inventory.Node, error) {
	fullURL := creds.URL
	if !strings.HasPrefix(fullURL, "https://") && !strings.HasPrefix(fullURL, "http://") {
		fullURL = "https://" + fullURL
	}

	u, err := url.Parse(fullURL)
	if err != nil {
		return nil, err
	}
	u.User = url.UserPassword(creds.Username, creds.Password)

	log.Infof("Connecting to vCenter at %s (auto-discover mode)", creds.URL)
	c, err := govmomi.NewClient(ctx, u, true)
	if err != nil {
		return nil, err
	}
	defer logout(ctx, c)

	finder := find.NewFinder(c.Client, true)

	// If datacenter is specified, use it; otherwise auto-discover
	var dc *object.Datacenter
	if creds.Datacenter != "" {
		dc, err = finder.Datacenter(ctx, creds.Datacenter)
		if err != nil {
			return nil, fmt.Errorf("failed to find datacenter %s: %w", creds.Datacenter, err)
		}
	} else {
		// Auto-discover: get the default datacenter
		dc, err = finder.DefaultDatacenter(ctx)
		if err != nil {
			// Try listing all datacenters
			dcs, listErr := finder.DatacenterList(ctx, "*")
			if listErr != nil || len(dcs) == 0 {
				return nil, fmt.Errorf("no datacenter found: %v", err)
			}
			dc = dcs[0]
		}
	}

	finder.SetDatacenter(dc)

	rootNode := &inventory.Node{
		Name: dc.Name(),
		Type: "datacenter",
	}

	folders, err := dc.Folders(ctx)
	if err != nil {
		return nil, err
	}

	rootFolder := object.NewFolder(c.Client, folders.VmFolder.Reference())
	children, err := rootFolder.Children(ctx)
	if err != nil {
		return nil, err
	}

	for _, child := range children {
		node, err := processEntity(ctx, c, child, "")
		if err != nil {
			log.Warnf("Could not process entity %s: %v", child.Reference().Value, err)
			continue
		}
		if node != nil {
			rootNode.Children = append(rootNode.Children, *node)
		}
	}

	log.Debugf("Constructed vCenter inventory tree (auto-discover): %+v", rootNode)
	return rootNode, nil
}

// logout releases a vCenter session. A failure only means the session expires on
// its own, so it is logged at debug level rather than returned.
func logout(ctx context.Context, c interface{ Logout(context.Context) error }) {
	if err := c.Logout(ctx); err != nil {
		log.Debugf("vCenter logout failed: %v", err)
	}
}

// ErrUnsupportedOperation is returned by PowerOpVM for an operation other than
// on, off, reset or shutdown. The message is "unsupported power operation: <op>".
var ErrUnsupportedOperation = errors.New("unsupported power operation")

// DeviceNotFoundError is returned when a VM has no device with the given key.
type DeviceNotFoundError struct{ Key int32 }

func (e *DeviceNotFoundError) Error() string {
	return fmt.Sprintf("device with key %d not found", e.Key)
}

// IsNotFound reports whether err means that something the caller named (a VM, a
// datacenter, a device key) does not exist, as opposed to vCenter being unreachable
// or refusing the login.
func IsNotFound(err error) bool {
	var nf *find.NotFoundError
	var dnf *DeviceNotFoundError
	return errors.As(err, &nf) || errors.As(err, &dnf)
}

// HTTPStatus maps an error from a vCenter-backed handler to an HTTP status.
// Kubernetes API errors raised while resolving credentials keep the API server's
// status (they arrive wrapped); an unsupported operation is the caller's mistake
// (400) and an unknown VM, datacenter or device is a 404. Everything else (vCenter
// unreachable, login refused, a failed task) is 500 on purpose: it is the
// upstream's failure, but a 502 could be replaced by an intermediary's own error
// page and hide the message from the UI.
func HTTPStatus(err error) int {
	code := httpx.StatusFor(err)
	if code != http.StatusInternalServerError {
		return code
	}
	switch {
	case errors.Is(err, ErrUnsupportedOperation):
		return http.StatusBadRequest
	case IsNotFound(err):
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
