// pkg/main.go
package main

import (
	"mime"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

func main() {
	// The same binary runs in two modes: the API server, and the worker that
	// performs one export inside a Kubernetes Job. Keeping them in one binary
	// means one image, one version and one build to keep in sync.
	if len(os.Args) > 1 && os.Args[1] == "export-worker" {
		log.SetFormatter(&log.JSONFormatter{})
		if lvl, err := log.ParseLevel(os.Getenv("LOG_LEVEL")); err == nil {
			log.SetLevel(lvl)
		}
		os.Exit(RunExportWorker())
	}
	// Removes one export's files from an export volume, inside a short-lived Job
	// in the export's namespace (see export_cleanup.go).
	if len(os.Args) > 1 && os.Args[1] == "export-cleanup" {
		log.SetFormatter(&log.JSONFormatter{})
		os.Exit(RunExportCleanup())
	}

	// Fix MIME types for serving static files
	mime.AddExtensionType(".js", "application/javascript")
	mime.AddExtensionType(".css", "text/css")
	mime.AddExtensionType(".html", "text/html")
	mime.AddExtensionType(".json", "application/json")
	mime.AddExtensionType(".svg", "image/svg+xml")
	mime.AddExtensionType(".ico", "image/x-icon")

	log.SetFormatter(&log.JSONFormatter{})

	logLevel, err := log.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		logLevel = log.InfoLevel
	}
	log.SetLevel(logLevel)

	log.Infof("Starting VM Import UI Backend v%s", appVersion)

	provider, err := NewK8sProvider()
	if err != nil && os.Getenv("USE_MOCK_DATA") != "true" {
		log.Fatalf("Failed to create Kubernetes clients: %v", err)
	}
	if provider == nil {
		provider = &K8sProvider{mode: authServiceAccount}
	}

	router := mux.NewRouter()
	api := router.PathPrefix("/api/v1").Subrouter()

	// API Handlers
	api.HandleFunc("/capabilities", userScoped(provider, GetCapabilitiesHandler)).Methods("GET")
	api.HandleFunc("/support-bundle", userScoped(provider, SupportBundleHandler)).Methods("GET")
	api.HandleFunc("/vcenter/inventory/{namespace}/{name}", userScoped(provider, HandleGetInventory)).Methods("GET")
	api.HandleFunc("/vcenter/vm/{namespace}/{name}/power", userScoped(provider, HandleVMPowerOp)).Methods("POST")
	api.HandleFunc("/vcenter/vm/{namespace}/{name}/rename", userScoped(provider, HandleVMRename)).Methods("POST")
	api.HandleFunc("/vcenter/vm/{namespace}/{name}/mac", userScoped(provider, HandleUpdateVMMAC)).Methods("POST")
	api.HandleFunc("/plans", userScoped(provider, CreatePlanHandler)).Methods("POST")
	api.HandleFunc("/plans", userScoped(provider, ListPlansHandler)).Methods("GET")
	api.HandleFunc("/plans/{namespace}/{name}", userScoped(provider, UpdatePlanHandler)).Methods("PUT")
	api.HandleFunc("/plans/{namespace}/{name}", userScoped(provider, DeletePlanHandler)).Methods("DELETE")
	api.HandleFunc("/plans/{namespace}/{name}/run", userScoped(provider, RunPlanHandler)).Methods("POST")
	api.HandleFunc("/plans/{namespace}/{name}/logs", userScoped(provider, HandleGetPlanLogs)).Methods("GET")
	api.HandleFunc("/plans/{namespace}/{name}/yaml", userScoped(provider, HandleGetPlanYAML)).Methods("GET")

	// Harvester Resource Handlers
	api.HandleFunc("/harvester/vmwaresources", userScoped(provider, ListVmwareSourcesHandler)).Methods("GET")
	api.HandleFunc("/harvester/vmwaresources", userScoped(provider, CreateVmwareSourceHandler)).Methods("POST")
	api.HandleFunc("/harvester/vmwaresources/{namespace}/{name}", userScoped(provider, GetVmwareSourceDetails)).Methods("GET")
	api.HandleFunc("/harvester/vmwaresources/{namespace}/{name}", userScoped(provider, UpdateVmwareSourceHandler)).Methods("PUT")
	api.HandleFunc("/harvester/vmwaresources/{namespace}/{name}", userScoped(provider, DeleteVmwareSourceHandler)).Methods("DELETE")
	api.HandleFunc("/harvester/vmwaresources/{namespace}/{name}/yaml", userScoped(provider, func(c *K8sClients) http.HandlerFunc { return HandleGetSourceYAML(c, vmwareSourceGVR) })).Methods("GET")

	api.HandleFunc("/harvester/ovasources", userScoped(provider, ListOvaSourcesHandler)).Methods("GET")
	api.HandleFunc("/harvester/ovasources", userScoped(provider, CreateOvaSourceHandler)).Methods("POST")
	api.HandleFunc("/harvester/ovasources/{namespace}/{name}", userScoped(provider, GetOvaSourceDetails)).Methods("GET")
	api.HandleFunc("/harvester/ovasources/{namespace}/{name}", userScoped(provider, UpdateOvaSourceHandler)).Methods("PUT")
	api.HandleFunc("/harvester/ovasources/{namespace}/{name}", userScoped(provider, DeleteOvaSourceHandler)).Methods("DELETE")
	api.HandleFunc("/harvester/ovasources/{namespace}/{name}/yaml", userScoped(provider, func(c *K8sClients) http.HandlerFunc { return HandleGetSourceYAML(c, ovaSourceGVR) })).Methods("GET")

	api.HandleFunc("/harvester/namespaces", userScoped(provider, ListNamespacesHandler)).Methods("GET")
	api.HandleFunc("/harvester/namespaces", userScoped(provider, CreateNamespaceHandler)).Methods("POST")
	api.HandleFunc("/harvester/vlanconfigs", userScoped(provider, ListVlanConfigsHandler)).Methods("GET")
	api.HandleFunc("/harvester/storageclasses", userScoped(provider, ListStorageClassesHandler)).Methods("GET")
	api.HandleFunc("/harvester/virtualmachines/{namespace}", userScoped(provider, ListVMsHandler)).Methods("GET")
	// Cluster-wide Harvester VM inventory, for the VM Export page.
	api.HandleFunc("/harvester/inventory", userScoped(provider, HandleGetHarvesterInventory)).Methods("GET")
	api.HandleFunc("/exports/preview", userScoped(provider, PreviewOVFHandler)).Methods("POST")
	api.HandleFunc("/exports", userScoped(provider, ListExportsHandler)).Methods("GET")
	api.HandleFunc("/exports", userScoped(provider, CreateExportHandler)).Methods("POST")
	api.HandleFunc("/exports/{namespace}/{id}", userScoped(provider, GetExportHandler)).Methods("GET")
	api.HandleFunc("/exports/{namespace}/{id}", userScoped(provider, DeleteExportHandler)).Methods("DELETE")
	api.HandleFunc("/exports/{namespace}/{id}/logs", userScoped(provider, GetExportLogsHandler)).Methods("GET")
	api.HandleFunc("/exports/{namespace}/{id}/download", userScoped(provider, DownloadExportHandler)).Methods("GET")

	// Forklift Handlers
	api.HandleFunc("/forklift/availability", userScoped(provider, CheckForkliftAvailability)).Methods("GET")
	api.HandleFunc("/forklift/providers", userScoped(provider, ListForkliftProvidersHandler)).Methods("GET")
	api.HandleFunc("/forklift/providers", userScoped(provider, CreateForkliftProviderHandler)).Methods("POST")
	api.HandleFunc("/forklift/providers/{namespace}/{name}", userScoped(provider, GetForkliftProviderDetails)).Methods("GET")
	api.HandleFunc("/forklift/providers/{namespace}/{name}", userScoped(provider, UpdateForkliftProviderHandler)).Methods("PUT")
	api.HandleFunc("/forklift/providers/{namespace}/{name}", userScoped(provider, DeleteForkliftProviderHandler)).Methods("DELETE")
	api.HandleFunc("/forklift/providers/{namespace}/{name}/yaml", userScoped(provider, func(c *K8sClients) http.HandlerFunc { return HandleGetSourceYAML(c, forkliftProviderGVR) })).Methods("GET")
	api.HandleFunc("/forklift/inventory/{namespace}/{name}", userScoped(provider, HandleGetForkliftInventory)).Methods("GET")
	api.HandleFunc("/forklift/inventory/ova/{namespace}/{name}/{resource}", userScoped(provider, HandleGetForkliftOvaInventory)).Methods("GET")
	api.HandleFunc("/forklift/plans", userScoped(provider, ListForkliftPlansHandler)).Methods("GET")
	api.HandleFunc("/forklift/plans", userScoped(provider, CreateForkliftPlanHandler)).Methods("POST")
	api.HandleFunc("/forklift/plans/{namespace}/{name}", userScoped(provider, DeleteForkliftPlanHandler)).Methods("DELETE")
	api.HandleFunc("/forklift/plans/{namespace}/{name}/logs", userScoped(provider, HandleGetForkliftLogs)).Methods("GET")
	api.HandleFunc("/forklift/plans/{namespace}/{name}/yaml", userScoped(provider, HandleGetForkliftPlanYAML)).Methods("GET")
	api.HandleFunc("/forklift/plans/{namespace}/{name}/run", userScoped(provider, CreateForkliftMigrationHandler)).Methods("POST")
	api.HandleFunc("/forklift/plans/{namespace}/{name}/migration", userScoped(provider, GetForkliftMigrationStatus)).Methods("GET")
	api.HandleFunc("/forklift/plans/{namespace}/{name}/migration", userScoped(provider, DeleteForkliftMigrationHandler)).Methods("DELETE")
	api.HandleFunc("/forklift/networkmaps/{namespace}/{name}", userScoped(provider, func(c *K8sClients) http.HandlerFunc { return HandleGetResource(c, forkliftNetworkMapGVR) })).Methods("GET")
	api.HandleFunc("/forklift/networkmaps/{namespace}/{name}/yaml", userScoped(provider, func(c *K8sClients) http.HandlerFunc { return HandleGetSourceYAML(c, forkliftNetworkMapGVR) })).Methods("GET")
	api.HandleFunc("/forklift/storagemaps/{namespace}/{name}", userScoped(provider, func(c *K8sClients) http.HandlerFunc { return HandleGetResource(c, forkliftStorageMapGVR) })).Methods("GET")
	api.HandleFunc("/forklift/storagemaps/{namespace}/{name}/yaml", userScoped(provider, func(c *K8sClients) http.HandlerFunc { return HandleGetSourceYAML(c, forkliftStorageMapGVR) })).Methods("GET")
	api.HandleFunc("/forklift/migrations/{namespace}/{name}/yaml", userScoped(provider, func(c *K8sClients) http.HandlerFunc { return HandleGetSourceYAML(c, forkliftMigrationGVR) })).Methods("GET")

	// Serve the frontend
	uiPath := "/ui"
	if p := os.Getenv("UI_PATH"); p != "" {
		uiPath = p
	}
	fs := http.FileServer(http.Dir(uiPath))
	router.PathPrefix("/").Handler(http.StripPrefix("/", fs))

	log.Info("Server is starting on port 8080")
	if err := http.ListenAndServe(":8080", recoverMiddleware(router)); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// recoverMiddleware turns a panic in any handler into a 500 response instead of
// crashing the connection (which the client sees as a hung/empty reply). It keeps
// the server responsive when a dependency is unavailable — e.g. running with
// USE_MOCK_DATA and no cluster, where the Kubernetes clients are nil.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Errorf("panic serving %s %s: %v", r.Method, r.URL.Path, rec)
				respondWithError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
