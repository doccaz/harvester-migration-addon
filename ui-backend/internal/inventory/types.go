// types.go
package inventory

// Disk represents a virtual disk in vCenter or Harvester.
// The trailing fields are Harvester-only and stay empty for vCenter inventory.
type Disk struct {
	Name     string `json:"name"`
	Capacity int64  `json:"capacity"` // in bytes
	BusType  string `json:"busType"`  // e.g. scsi, ide, sata, nvme, virtio
	UnitNum  int32  `json:"unitNum"`

	Kind         string `json:"kind,omitempty"`         // backing: pvc, cloudinit, container, other (Harvester)
	Device       string `json:"device,omitempty"`       // device type: disk, cdrom, lun (Harvester)
	PVCName      string `json:"pvcName,omitempty"`      // Harvester
	StorageClass string `json:"storageClass,omitempty"` // Harvester
	VolumeMode   string `json:"volumeMode,omitempty"`   // Block or Filesystem (Harvester)
	BootOrder    int32  `json:"bootOrder,omitempty"`    // Harvester
}

// Network represents a network interface in vCenter
type Network struct {
	Name string `json:"name"`
	ID   string `json:"id"`
	MAC  string `json:"mac"`
	Key  int32  `json:"key"`
}

// Node represents a generic node in the vCenter inventory tree.
type Node struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Type          string    `json:"type"`
	Children      []Node    `json:"children,omitempty"`
	Networks      []Network `json:"networks,omitempty"`
	Disks         []Disk    `json:"disks,omitempty"`
	CPU           int32     `json:"cpu,omitempty"`
	MemoryMB      int32     `json:"memoryMB,omitempty"`
	DiskSizeGB    int64     `json:"diskSizeGB,omitempty"`
	Folder        string    `json:"folder,omitempty"`
	PowerState    string    `json:"powerState,omitempty"`
	DatastoreID   string    `json:"datastoreId,omitempty"`
	DatastoreName string    `json:"datastoreName,omitempty"`

	// Harvester-only fields; empty for vCenter inventory.
	Namespace      string   `json:"namespace,omitempty"`
	Architecture   string   `json:"architecture,omitempty"`
	Firmware       string   `json:"firmware,omitempty"` // bios or efi
	MachineType    string   `json:"machineType,omitempty"`
	RunStrategy    string   `json:"runStrategy,omitempty"`
	ExportBlockers []string `json:"exportBlockers,omitempty"` // why this VM cannot be exported now

	// Warnings is set on the inventory root only: things that could not be read, which
	// make parts of the tree less complete than they look (unknown disk sizes, VMs
	// treated as running).
	Warnings []string `json:"warnings,omitempty"`
}
