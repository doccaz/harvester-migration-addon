// routes.go
package main

import (
	"net/http"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/forklift"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/vmic"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/harvester"

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
	api.HandleFunc("/vcenter/inventory/{namespace}/{name}", kube.Scoped(provider, vmic.GetInventory)).Methods("GET")
	api.HandleFunc("/vcenter/vm/{namespace}/{name}/power", kube.Scoped(provider, vmic.PowerOp)).Methods("POST")
	api.HandleFunc("/vcenter/vm/{namespace}/{name}/rename", kube.Scoped(provider, vmic.RenameVM)).Methods("POST")
	api.HandleFunc("/vcenter/vm/{namespace}/{name}/mac", kube.Scoped(provider, vmic.UpdateMAC)).Methods("POST")
	api.HandleFunc("/plans", kube.Scoped(provider, vmic.CreatePlan)).Methods("POST")
	api.HandleFunc("/plans", kube.Scoped(provider, vmic.ListPlans)).Methods("GET")
	api.HandleFunc("/plans/{namespace}/{name}", kube.Scoped(provider, vmic.UpdatePlan)).Methods("PUT")
	api.HandleFunc("/plans/{namespace}/{name}", kube.Scoped(provider, vmic.DeletePlan)).Methods("DELETE")
	api.HandleFunc("/plans/{namespace}/{name}/run", kube.Scoped(provider, vmic.RunPlan)).Methods("POST")
	api.HandleFunc("/plans/{namespace}/{name}/logs", kube.Scoped(provider, vmic.GetPlanLogs)).Methods("GET")
	api.HandleFunc("/plans/{namespace}/{name}/yaml", kube.Scoped(provider, vmic.GetPlanYAML)).Methods("GET")

	// Harvester Resource Handlers
	api.HandleFunc("/harvester/vmwaresources", kube.Scoped(provider, vmic.ListVmwareSources)).Methods("GET")
	api.HandleFunc("/harvester/vmwaresources", kube.Scoped(provider, vmic.CreateVmwareSource)).Methods("POST")
	api.HandleFunc("/harvester/vmwaresources/{namespace}/{name}", kube.Scoped(provider, vmic.GetVmwareSource)).Methods("GET")
	api.HandleFunc("/harvester/vmwaresources/{namespace}/{name}", kube.Scoped(provider, vmic.UpdateVmwareSource)).Methods("PUT")
	api.HandleFunc("/harvester/vmwaresources/{namespace}/{name}", kube.Scoped(provider, vmic.DeleteVmwareSource)).Methods("DELETE")
	api.HandleFunc("/harvester/vmwaresources/{namespace}/{name}/yaml", kube.Scoped(provider, func(c *kube.Clients) http.HandlerFunc { return harvester.GetSourceYAML(c, kube.VMwareSourceGVR) })).Methods("GET")

	api.HandleFunc("/harvester/ovasources", kube.Scoped(provider, vmic.ListOvaSources)).Methods("GET")
	api.HandleFunc("/harvester/ovasources", kube.Scoped(provider, vmic.CreateOvaSource)).Methods("POST")
	api.HandleFunc("/harvester/ovasources/{namespace}/{name}", kube.Scoped(provider, vmic.GetOvaSource)).Methods("GET")
	api.HandleFunc("/harvester/ovasources/{namespace}/{name}", kube.Scoped(provider, vmic.UpdateOvaSource)).Methods("PUT")
	api.HandleFunc("/harvester/ovasources/{namespace}/{name}", kube.Scoped(provider, vmic.DeleteOvaSource)).Methods("DELETE")
	api.HandleFunc("/harvester/ovasources/{namespace}/{name}/yaml", kube.Scoped(provider, func(c *kube.Clients) http.HandlerFunc { return harvester.GetSourceYAML(c, kube.OVASourceGVR) })).Methods("GET")

	api.HandleFunc("/harvester/namespaces", kube.Scoped(provider, harvester.ListNamespaces)).Methods("GET")
	api.HandleFunc("/harvester/namespaces", kube.Scoped(provider, harvester.CreateNamespace)).Methods("POST")
	api.HandleFunc("/harvester/vlanconfigs", kube.Scoped(provider, harvester.ListVlanConfigs)).Methods("GET")
	api.HandleFunc("/harvester/storageclasses", kube.Scoped(provider, harvester.ListStorageClasses)).Methods("GET")
	api.HandleFunc("/harvester/virtualmachines/{namespace}", kube.Scoped(provider, harvester.ListVMs)).Methods("GET")
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
	api.HandleFunc("/forklift/availability", kube.Scoped(provider, forklift.CheckAvailability)).Methods("GET")
	api.HandleFunc("/forklift/providers", kube.Scoped(provider, forklift.ListProviders)).Methods("GET")
	api.HandleFunc("/forklift/providers", kube.Scoped(provider, forklift.CreateProvider)).Methods("POST")
	api.HandleFunc("/forklift/providers/{namespace}/{name}", kube.Scoped(provider, forklift.GetProvider)).Methods("GET")
	api.HandleFunc("/forklift/providers/{namespace}/{name}", kube.Scoped(provider, forklift.UpdateProvider)).Methods("PUT")
	api.HandleFunc("/forklift/providers/{namespace}/{name}", kube.Scoped(provider, forklift.DeleteProvider)).Methods("DELETE")
	api.HandleFunc("/forklift/providers/{namespace}/{name}/yaml", kube.Scoped(provider, func(c *kube.Clients) http.HandlerFunc { return harvester.GetSourceYAML(c, kube.ForkliftProviderGVR) })).Methods("GET")
	api.HandleFunc("/forklift/inventory/{namespace}/{name}", kube.Scoped(provider, forklift.GetInventory)).Methods("GET")
	api.HandleFunc("/forklift/inventory/ova/{namespace}/{name}/{resource}", kube.Scoped(provider, forklift.GetOvaInventory)).Methods("GET")
	api.HandleFunc("/forklift/plans", kube.Scoped(provider, forklift.ListPlans)).Methods("GET")
	api.HandleFunc("/forklift/plans", kube.Scoped(provider, forklift.CreatePlan)).Methods("POST")
	api.HandleFunc("/forklift/plans/{namespace}/{name}", kube.Scoped(provider, forklift.DeletePlan)).Methods("DELETE")
	api.HandleFunc("/forklift/plans/{namespace}/{name}/logs", kube.Scoped(provider, forklift.GetLogs)).Methods("GET")
	api.HandleFunc("/forklift/plans/{namespace}/{name}/yaml", kube.Scoped(provider, forklift.GetPlanYAML)).Methods("GET")
	api.HandleFunc("/forklift/plans/{namespace}/{name}/run", kube.Scoped(provider, forklift.CreateMigration)).Methods("POST")
	api.HandleFunc("/forklift/plans/{namespace}/{name}/migration", kube.Scoped(provider, forklift.GetMigrationStatus)).Methods("GET")
	api.HandleFunc("/forklift/plans/{namespace}/{name}/migration", kube.Scoped(provider, forklift.DeleteMigration)).Methods("DELETE")
	api.HandleFunc("/forklift/networkmaps/{namespace}/{name}", kube.Scoped(provider, func(c *kube.Clients) http.HandlerFunc { return harvester.GetResource(c, kube.ForkliftNetworkMapGVR) })).Methods("GET")
	api.HandleFunc("/forklift/networkmaps/{namespace}/{name}/yaml", kube.Scoped(provider, func(c *kube.Clients) http.HandlerFunc { return harvester.GetSourceYAML(c, kube.ForkliftNetworkMapGVR) })).Methods("GET")
	api.HandleFunc("/forklift/storagemaps/{namespace}/{name}", kube.Scoped(provider, func(c *kube.Clients) http.HandlerFunc { return harvester.GetResource(c, kube.ForkliftStorageMapGVR) })).Methods("GET")
	api.HandleFunc("/forklift/storagemaps/{namespace}/{name}/yaml", kube.Scoped(provider, func(c *kube.Clients) http.HandlerFunc { return harvester.GetSourceYAML(c, kube.ForkliftStorageMapGVR) })).Methods("GET")
	api.HandleFunc("/forklift/migrations/{namespace}/{name}/yaml", kube.Scoped(provider, func(c *kube.Clients) http.HandlerFunc { return harvester.GetSourceYAML(c, kube.ForkliftMigrationGVR) })).Methods("GET")

	// Serve the frontend
	fs := http.FileServer(http.Dir(uiPath))
	router.PathPrefix("/").Handler(http.StripPrefix("/", fs))
	return router
}
