// forklift_inventory.go
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/httpx"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HandleGetForkliftInventory fetches vCenter inventory using Forklift Provider credentials
func HandleGetForkliftInventory(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		log.Infof("Fetching inventory for Forklift Provider %s/%s", namespace, name)

		providerObj, err := clients.Dynamic.Resource(kube.ForkliftProviderGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to get Forklift Provider: "+err.Error())
			return
		}

		providerURL, found := kube.NestedStringOrWarn(providerObj.Object, "spec", "url")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Forklift Provider missing URL")
			return
		}
		secretName, found := kube.NestedStringOrWarn(providerObj.Object, "spec", "secret", "name")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Forklift Provider missing secret name")
			return
		}
		secretNamespace, found := kube.NestedStringOrWarn(providerObj.Object, "spec", "secret", "namespace")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Forklift Provider missing secret namespace")
			return
		}

		secret, err := clients.Clientset.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to get Forklift credentials secret: "+err.Error())
			return
		}

		// Forklift secrets use "user" and "password" fields, and "url"
		// The URL from the secret or Provider spec both work; use Provider spec URL
		// Pass the full URL including /sdk path, same as VM Import Controller
		creds := VCenterCredentials{
			URL:        providerURL,
			Username:   string(secret.Data["user"]),
			Password:   string(secret.Data["password"]),
			Datacenter: "", // Will be auto-discovered
		}

		inventory, err := GetVCenterInventoryAutoDiscover(r.Context(), creds)
		if err != nil {
			log.Errorf("Failed to get vCenter inventory via Forklift Provider: %v", err)
			httpx.RespondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusOK, inventory)
	}
}

// inventoryClient bounds calls to the in-cluster forklift-inventory service; the
// default client would wait forever on a hung upstream.
var inventoryClient = &http.Client{Timeout: 60 * time.Second}

// ovaInventoryResources are the forklift-inventory collections the UI reads.
var ovaInventoryResources = map[string]bool{"vms": true, "networks": true, "disks": true}

// HandleGetForkliftOvaInventory proxies inventory requests for OVA providers through the
// forklift-inventory service. OVA providers auto-deploy an OVA server pod that scans
// NFS shares for OVF/OVA files. The inventory service exposes VMs, networks, and disks.
func HandleGetForkliftOvaInventory(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]
		// resource can be "vms", "networks", or "disks"
		resource := vars["resource"]
		if resource == "" {
			resource = "vms"
		}
		// The value is spliced into the inventory URL, so only known resources
		// are accepted; anything else could address other inventory paths.
		if !ovaInventoryResources[resource] {
			httpx.RespondWithError(w, http.StatusBadRequest, "Unsupported OVA inventory resource: "+resource)
			return
		}

		// 1. Get the Provider CR to obtain its UID
		providerObj, err := clients.Dynamic.Resource(kube.ForkliftProviderGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusNotFound, "Failed to get OVA provider: "+err.Error())
			return
		}
		providerUID := string(providerObj.GetUID())

		// 2. Discover the forklift-inventory service
		// The inventory service runs in the same namespace as the Forklift operator
		forkliftNs := namespace
		svc, err := clients.Clientset.CoreV1().Services(forkliftNs).Get(context.TODO(), "forklift-inventory", metav1.GetOptions{})
		if err != nil {
			// Try the default forklift namespace
			forkliftNs = "forklift"
			svc, err = clients.Clientset.CoreV1().Services(forkliftNs).Get(context.TODO(), "forklift-inventory", metav1.GetOptions{})
			if err != nil {
				httpx.RespondWithError(w, http.StatusInternalServerError, "Cannot find forklift-inventory service: "+err.Error())
				return
			}
		}

		// 3. Build the inventory URL
		// The inventory service typically listens on port 8443
		port := "8443"
		for _, p := range svc.Spec.Ports {
			if p.Name == "api" || p.Name == "https" {
				port = fmt.Sprintf("%d", p.Port)
				break
			}
		}
		inventoryURL := fmt.Sprintf("http://forklift-inventory.%s.svc:%s/providers/ova/%s/%s",
			forkliftNs, port, providerUID, resource)

		log.Debugf("Proxying OVA inventory request to: %s", inventoryURL)

		// 4. Proxy the request
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, inventoryURL, nil)
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to build inventory request: "+err.Error())
			return
		}
		resp, err := inventoryClient.Do(req)
		if err != nil {
			httpx.RespondWithError(w, http.StatusBadGateway, "Failed to reach forklift-inventory: "+err.Error())
			return
		}
		defer resp.Body.Close()

		// Copy response headers and status
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		if _, copyErr := io.Copy(w, resp.Body); copyErr != nil {
			log.Warnf("Failed to proxy OVA inventory response: %v", copyErr)
		}
	}
}
