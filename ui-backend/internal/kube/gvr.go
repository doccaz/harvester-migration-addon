// gvr.go
package kube

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	VMIGVR = schema.GroupVersionResource{
		Group:    "migration.harvesterhci.io",
		Version:  "v1beta1",
		Resource: "virtualmachineimports",
	}
	VMwareSourceGVR = schema.GroupVersionResource{
		Group:    "migration.harvesterhci.io",
		Version:  "v1beta1",
		Resource: "vmwaresources",
	}
	OVASourceGVR = schema.GroupVersionResource{
		Group:    "migration.harvesterhci.io",
		Version:  "v1beta1",
		Resource: "ovasources",
	}
	VMGVR = schema.GroupVersionResource{
		Group:    "kubevirt.io",
		Version:  "v1",
		Resource: "virtualmachines",
	}
	// NEW: To check cluster version
	SettingsGVR = schema.GroupVersionResource{
		Group:    "harvesterhci.io",
		Version:  "v1beta1",
		Resource: "settings",
	}

	// Forklift GVRs
	ForkliftProviderGVR = schema.GroupVersionResource{
		Group:    "forklift.konveyor.io",
		Version:  "v1beta1",
		Resource: "providers",
	}
	ForkliftPlanGVR = schema.GroupVersionResource{
		Group:    "forklift.konveyor.io",
		Version:  "v1beta1",
		Resource: "plans",
	}
	ForkliftNetworkMapGVR = schema.GroupVersionResource{
		Group:    "forklift.konveyor.io",
		Version:  "v1beta1",
		Resource: "networkmaps",
	}
	ForkliftStorageMapGVR = schema.GroupVersionResource{
		Group:    "forklift.konveyor.io",
		Version:  "v1beta1",
		Resource: "storagemaps",
	}
	ForkliftMigrationGVR = schema.GroupVersionResource{
		Group:    "forklift.konveyor.io",
		Version:  "v1beta1",
		Resource: "migrations",
	}
)
