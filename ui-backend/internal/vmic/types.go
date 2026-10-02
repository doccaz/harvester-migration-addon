// types.go
package vmic

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The JSON tags MUST match the actual CRD field names from the Harvester documentation.
type VirtualMachineImportSpec struct {
	VirtualMachineName string           `json:"virtualMachineName"`
	SourceCluster      SourceCluster    `json:"sourceCluster"`
	NetworkMapping     []NetworkMapping `json:"networkMapping,omitempty"`
	StorageClass       string           `json:"storageClass,omitempty"`
	Schedule           *metav1.Time     `json:"schedule,omitempty"`

	// New fields for folder support and advanced options
	Folder                         string `json:"folder,omitempty"`
	ForcePowerOff                  *bool  `json:"forcePowerOff,omitempty"`
	GracefulShutdownTimeoutSeconds int    `json:"gracefulShutdownTimeoutSeconds,omitempty"`
	DefaultNetworkInterfaceModel   string `json:"defaultNetworkInterfaceModel,omitempty"`

	// New fields for Harvester v1.6+
	SkipPreflightChecks *bool  `json:"skipPreflightChecks,omitempty"`
	DefaultDiskBusType  string `json:"defaultDiskBusType,omitempty"`
}

type SourceCluster struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
}

type NetworkMapping struct {
	SourceNetwork      string `json:"sourceNetwork"`
	DestinationNetwork string `json:"destinationNetwork"`
	// New field for per-interface model selection
	NetworkInterfaceModel string `json:"networkInterfaceModel,omitempty"`
}

// VirtualMachineImportStatus defines the observed state of VirtualMachineImport
type VirtualMachineImportStatus struct {
	Conditions   []metav1.Condition `json:"conditions,omitempty"`
	ImportStatus string             `json:"importStatus,omitempty"`
}

// UpdatePlanPayload is the JSON payload for patching editable fields on a VMIC plan.
// Every field is a pointer so a nil value means "leave unchanged", while a provided
// value (including empty string / false / 0) is applied — empty strings and zero
// timeouts clear the corresponding spec field.
type UpdatePlanPayload struct {
	VirtualMachineName             *string `json:"virtualMachineName,omitempty"`
	StorageClass                   *string `json:"storageClass,omitempty"`
	Folder                         *string `json:"folder,omitempty"`
	ForcePowerOff                  *bool   `json:"forcePowerOff,omitempty"`
	GracefulShutdownTimeoutSeconds *int64  `json:"gracefulShutdownTimeoutSeconds,omitempty"`
	DefaultNetworkInterfaceModel   *string `json:"defaultNetworkInterfaceModel,omitempty"`
	SkipPreflightChecks            *bool   `json:"skipPreflightChecks,omitempty"`
	DefaultDiskBusType             *string `json:"defaultDiskBusType,omitempty"`
	// NetworkMapping, when non-nil, replaces spec.networkMapping wholesale. An empty
	// slice clears it. VMIC marks a plan invalid if a source NIC is left unmapped.
	NetworkMapping *[]NetworkMapping `json:"networkMapping,omitempty"`
}

// VirtualMachineImport is the Schema for the virtualmachineimports API
type VirtualMachineImport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineImportSpec   `json:"spec,omitempty"`
	Status VirtualMachineImportStatus `json:"status,omitempty"`
}

// --- Forklift Types ---
