// routes.go
package main

import (
	"net/http"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/capabilities"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/inventory"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"

	"github.com/gorilla/mux"
)

// newRouter wires every API route and the static frontend. It is separate from
// main() so a test can enumerate the routes: the REST API is the contract with the
// frontend, and a route dropped during a refactor must fail a test.
func newRouter(provider *kube.Provider, uiPath string) *mux.Router {
	router := mux.NewRouter()
	api := router.PathPrefix("/api/v1").Subrouter()

	// API Handlers
	api.HandleFunc("/capabilities", kube.Scoped(provider, capabilities.Handler)).Methods("GET")
	api.HandleFunc("/support-bundle", kube.Scoped(provider, SupportBundleHandler)).Methods("GET")
	api.HandleFunc("/vcenter/inventory/{namespace}/{name}", kube.Scoped(provider, HandleGetInventory)).Methods("GET")
	api.HandleFunc("/vcenter/vm/{namespace}/{name}/power", kube.Scoped(provider, HandleVMPowerOp)).Methods("POST")
	api.HandleFunc("/vcenter/vm/{namespace}/{name}/rename", kube.Scoped(provider, HandleVMRename)).Methods("POST")
	api.HandleFunc("/vcenter/vm/{namespace}/{name}/mac", kube.Scoped(provider, HandleUpdateVMMAC)).Methods("POST")
	api.HandleFunc("/plans", kube.Scoped(provider, CreatePlanHandler)).Methods("POST")
	api.HandleFunc("/plans", kube.Scoped(provider, ListPlansHandler)).Methods("GET")
	api.HandleFunc("/plans/{namespace}/{name}", kube.Scoped(provider, UpdatePlanHandler)).Methods("PUT")
	api.HandleFunc("/plans/{namespace}/{name}", kube.Scoped(provider, DeletePlanHandler)).Methods("DELETE")
	api.HandleFunc("/plans/{namespace}/{name}/run", kube.Scoped(provider, RunPlanHandler)).Methods("POST")
	api.HandleFunc("/plans/{namespace}/{name}/logs", kube.Scoped(provider, HandleGetPlanLogs)).Methods("GET")
	api.HandleFunc("/plans/{namespace}/{name}/yaml", kube.Scoped(provider, HandleGetPlanYAML)).Methods("GET")

	// Harvester Resource Handlers
	api.HandleFunc("/harvester/vmwaresources", kube.Scoped(provider, ListVmwareSourcesHandler)).Methods("GET")
	api.HandleFunc("/harvester/vmwaresources", kube.Scoped(provider, CreateVmwareSourceHandler)).Methods("POST")
	api.HandleFunc("/harvester/vmwaresources/{namespace}/{name}", kube.Scoped(provider, GetVmwareSourceDetails)).Methods("GET")
	api.HandleFunc("/harvester/vmwaresources/{namespace}/{name}", kube.Scoped(provider, UpdateVmwareSourceHandler)).Methods("PUT")
	api.HandleFunc("/harvester/vmwaresources/{namespace}/{name}", kube.Scoped(provider, DeleteVmwareSourceHandler)).Methods("DELETE")
	api.HandleFunc("/harvester/vmwaresources/{namespace}/{name}/yaml", kube.Scoped(provider, func(c *kube.Clients) http.HandlerFunc { return HandleGetSourceYAML(c, kube.VMwareSourceGVR) })).Methods("GET")

	api.HandleFunc("/harvester/ovasources", kube.Scoped(provider, ListOvaSourcesHandler)).Methods("GET")
	api.HandleFunc("/harvester/ovasources", kube.Scoped(provider, CreateOvaSourceHandler)).Methods("POST")
	api.HandleFunc("/harvester/ovasources/{namespace}/{name}", kube.Scoped(provider, GetOvaSourceDetails)).Methods("GET")
	api.HandleFunc("/harvester/ovasources/{namespace}/{name}", kube.Scoped(provider, UpdateOvaSourceHandler)).Methods("PUT")
	api.HandleFunc("/harvester/ovasources/{namespace}/{name}", kube.Scoped(provider, DeleteOvaSourceHandler)).Methods("DELETE")
	api.HandleFunc("/harvester/ovasources/{namespace}/{name}/yaml", kube.Scoped(provider, func(c *kube.Clients) http.HandlerFunc { return HandleGetSourceYAML(c, kube.OVASourceGVR) })).Methods("GET")

	api.HandleFunc("/harvester/namespaces", kube.Scoped(provider, ListNamespacesHandler)).Methods("GET")
	api.HandleFunc("/harvester/namespaces", kube.Scoped(provider, CreateNamespaceHandler)).Methods("POST")
	api.HandleFunc("/harvester/vlanconfigs", kube.Scoped(provider, ListVlanConfigsHandler)).Methods("GET")
	api.HandleFunc("/harvester/storageclasses", kube.Scoped(provider, ListStorageClassesHandler)).Methods("GET")
	api.HandleFunc("/harvester/virtualmachines/{namespace}", kube.Scoped(provider, ListVMsHandler)).Methods("GET")
	// Cluster-wide Harvester VM inventory, for the VM Export page.
	api.HandleFunc("/harvester/inventory", kube.Scoped(provider, inventory.HandleGetHarvesterInventory)).Methods("GET")
	api.HandleFunc("/exports/preview", kube.Scoped(provider, PreviewOVFHandler)).Methods("POST")
	api.HandleFunc("/exports", kube.Scoped(provider, ListExportsHandler)).Methods("GET")
	api.HandleFunc("/exports", kube.Scoped(provider, CreateExportHandler)).Methods("POST")
	api.HandleFunc("/exports/{namespace}/{id}", kube.Scoped(provider, GetExportHandler)).Methods("GET")
	api.HandleFunc("/exports/{namespace}/{id}", kube.Scoped(provider, DeleteExportHandler)).Methods("DELETE")
	api.HandleFunc("/exports/{namespace}/{id}/logs", kube.Scoped(provider, GetExportLogsHandler)).Methods("GET")
	api.HandleFunc("/exports/{namespace}/{id}/download", kube.Scoped(provider, DownloadExportHandler)).Methods("GET")

	// Forklift Handlers
	api.HandleFunc("/forklift/availability", kube.Scoped(provider, CheckForkliftAvailability)).Methods("GET")
	api.HandleFunc("/forklift/providers", kube.Scoped(provider, ListForkliftProvidersHandler)).Methods("GET")
	api.HandleFunc("/forklift/providers", kube.Scoped(provider, CreateForkliftProviderHandler)).Methods("POST")
	api.HandleFunc("/forklift/providers/{namespace}/{name}", kube.Scoped(provider, GetForkliftProviderDetails)).Methods("GET")
	api.HandleFunc("/forklift/providers/{namespace}/{name}", kube.Scoped(provider, UpdateForkliftProviderHandler)).Methods("PUT")
	api.HandleFunc("/forklift/providers/{namespace}/{name}", kube.Scoped(provider, DeleteForkliftProviderHandler)).Methods("DELETE")
	api.HandleFunc("/forklift/providers/{namespace}/{name}/yaml", kube.Scoped(provider, func(c *kube.Clients) http.HandlerFunc { return HandleGetSourceYAML(c, kube.ForkliftProviderGVR) })).Methods("GET")
	api.HandleFunc("/forklift/inventory/{namespace}/{name}", kube.Scoped(provider, HandleGetForkliftInventory)).Methods("GET")
	api.HandleFunc("/forklift/inventory/ova/{namespace}/{name}/{resource}", kube.Scoped(provider, HandleGetForkliftOvaInventory)).Methods("GET")
	api.HandleFunc("/forklift/plans", kube.Scoped(provider, ListForkliftPlansHandler)).Methods("GET")
	api.HandleFunc("/forklift/plans", kube.Scoped(provider, CreateForkliftPlanHandler)).Methods("POST")
	api.HandleFunc("/forklift/plans/{namespace}/{name}", kube.Scoped(provider, DeleteForkliftPlanHandler)).Methods("DELETE")
	api.HandleFunc("/forklift/plans/{namespace}/{name}/logs", kube.Scoped(provider, HandleGetForkliftLogs)).Methods("GET")
	api.HandleFunc("/forklift/plans/{namespace}/{name}/yaml", kube.Scoped(provider, HandleGetForkliftPlanYAML)).Methods("GET")
	api.HandleFunc("/forklift/plans/{namespace}/{name}/run", kube.Scoped(provider, CreateForkliftMigrationHandler)).Methods("POST")
	api.HandleFunc("/forklift/plans/{namespace}/{name}/migration", kube.Scoped(provider, GetForkliftMigrationStatus)).Methods("GET")
	api.HandleFunc("/forklift/plans/{namespace}/{name}/migration", kube.Scoped(provider, DeleteForkliftMigrationHandler)).Methods("DELETE")
	api.HandleFunc("/forklift/networkmaps/{namespace}/{name}", kube.Scoped(provider, func(c *kube.Clients) http.HandlerFunc { return HandleGetResource(c, kube.ForkliftNetworkMapGVR) })).Methods("GET")
	api.HandleFunc("/forklift/networkmaps/{namespace}/{name}/yaml", kube.Scoped(provider, func(c *kube.Clients) http.HandlerFunc { return HandleGetSourceYAML(c, kube.ForkliftNetworkMapGVR) })).Methods("GET")
	api.HandleFunc("/forklift/storagemaps/{namespace}/{name}", kube.Scoped(provider, func(c *kube.Clients) http.HandlerFunc { return HandleGetResource(c, kube.ForkliftStorageMapGVR) })).Methods("GET")
	api.HandleFunc("/forklift/storagemaps/{namespace}/{name}/yaml", kube.Scoped(provider, func(c *kube.Clients) http.HandlerFunc { return HandleGetSourceYAML(c, kube.ForkliftStorageMapGVR) })).Methods("GET")
	api.HandleFunc("/forklift/migrations/{namespace}/{name}/yaml", kube.Scoped(provider, func(c *kube.Clients) http.HandlerFunc { return HandleGetSourceYAML(c, kube.ForkliftMigrationGVR) })).Methods("GET")

	// Serve the frontend
	fs := http.FileServer(http.Dir(uiPath))
	router.PathPrefix("/").Handler(http.StripPrefix("/", fs))
	return router
}
