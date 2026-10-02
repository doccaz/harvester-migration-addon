// pkg/main.go
package main

import (
	"mime"
	"net/http"
	"os"
	"time"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/export"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/httpx"

	log "github.com/sirupsen/logrus"
)

func main() {
	// The same binary runs in two modes: the API server, and the worker that
	// performs one export inside a Kubernetes Job. Keeping them in one binary
	// means one image, one version and one build to keep in sync.
	if len(os.Args) > 1 && os.Args[1] == export.WorkerArg {
		log.SetFormatter(&log.JSONFormatter{})
		if lvl, err := log.ParseLevel(os.Getenv("LOG_LEVEL")); err == nil {
			log.SetLevel(lvl)
		}
		os.Exit(export.RunWorker())
	}
	// Removes one export's files from an export volume, inside a short-lived Job
	// in the export's namespace (see export_cleanup.go).
	if len(os.Args) > 1 && os.Args[1] == export.CleanupArg {
		log.SetFormatter(&log.JSONFormatter{})
		os.Exit(export.RunCleanup())
	}

	// Fix MIME types for serving static files
	for ext, typ := range map[string]string{
		".js": "application/javascript", ".css": "text/css", ".html": "text/html",
		".json": "application/json", ".svg": "image/svg+xml", ".ico": "image/x-icon",
	} {
		if err := mime.AddExtensionType(ext, typ); err != nil {
			log.Warnf("could not register MIME type for %s: %v", ext, err)
		}
	}

	log.SetFormatter(&log.JSONFormatter{})

	logLevel, err := log.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		logLevel = log.InfoLevel
	}
	log.SetLevel(logLevel)

	log.Infof("Starting VM Import UI Backend v%s", appVersion)

	provider, err := kube.NewProvider()
	if err != nil && os.Getenv("USE_MOCK_DATA") != "true" {
		log.Fatalf("Failed to create Kubernetes clients: %v", err)
	}
	if provider == nil {
		provider = kube.NewServiceAccountProvider(nil)
	}

	uiPath := "/ui"
	if p := os.Getenv("UI_PATH"); p != "" {
		uiPath = p
	}
	router := newRouter(provider, uiPath)

	log.Info("Server is starting on port 8080")
	srv := &http.Server{
		Addr:    ":8080",
		Handler: recoverMiddleware(router),
		// Bounds slow-header clients. No write timeout: support bundles and
		// downloads can legitimately stream for a long time.
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
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
				httpx.RespondWithError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
