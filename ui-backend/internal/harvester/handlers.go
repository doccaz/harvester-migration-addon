// handlers.go

// Package harvester serves the read-mostly cluster lookups the UI needs next to a
// migration: namespaces, networks, storage classes and VMs, plus generic
// "get this resource as JSON/YAML" handlers that the engines' routes reuse.
package harvester

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/httpx"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

func ListNamespaces(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		namespaces, err := clients.Clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			httpx.RespondWithAPIErrorMsg(w, err, err.Error())
			return
		}
		httpx.RespondWithJSON(w, http.StatusOK, namespaces.Items)
	}
}

func CreateNamespace(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			httpx.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		log.Infof("Creating namespace: %s", payload.Name)
		nsSpec := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: payload.Name}}
		_, err := clients.Clientset.CoreV1().Namespaces().Create(context.TODO(), nsSpec, metav1.CreateOptions{})
		if err != nil {
			httpx.RespondWithAPIError(w, err)
			return
		}

		httpx.RespondWithJSON(w, http.StatusCreated, map[string]string{"status": "namespace created"})
	}
}

func ListVlanConfigs(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("Listing Harvester VlanConfigs")
		gvr := schema.GroupVersionResource{
			Group:    "k8s.cni.cncf.io",
			Version:  "v1",
			Resource: "network-attachment-definitions",
		}

		// Match any Harvester-managed network regardless of type. Filtering on
		// =L2VlanNetwork dropped UntaggedNetwork NADs (e.g. "local-network"); an
		// existence selector on the type label includes both VLAN and untagged.
		listOptions := metav1.ListOptions{
			LabelSelector: "network.harvesterhci.io/type",
		}

		list, err := clients.Dynamic.Resource(gvr).Namespace("").List(context.TODO(), listOptions)
		if err != nil {
			httpx.RespondWithAPIErrorMsg(w, err, err.Error())
			return
		}

		log.Debugf("Fetched VLAN definitions: %+v", list.Items)
		httpx.RespondWithJSON(w, http.StatusOK, list.Items)
	}
}

func ListStorageClasses(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scs, err := clients.Clientset.StorageV1().StorageClasses().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			httpx.RespondWithAPIErrorMsg(w, err, err.Error())
			return
		}
		httpx.RespondWithJSON(w, http.StatusOK, scs.Items)
	}
}

// GetResource returns a namespaced resource as JSON
func GetResource(clients *kube.Clients, gvr schema.GroupVersionResource) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		item, err := clients.Dynamic.Resource(gvr).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithAPIErrorMsg(w, err, err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusOK, item.Object)
	}
}

func GetSourceYAML(clients *kube.Clients, gvr schema.GroupVersionResource) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		log.Infof("Fetching YAML for source %s/%s", namespace, name)

		item, err := clients.Dynamic.Resource(gvr).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithAPIError(w, err)
			return
		}

		yamlBytes, err := yaml.Marshal(item.Object)
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to marshal source to YAML: "+err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/yaml")
		// Object names and values come from the cluster; never let a browser sniff
		// this body into something it would render.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(yamlBytes); err != nil { //nolint:gosec // G705: application/yaml + nosniff, and the requester's own object
			log.Warnf("Failed to write response: %v", err)
		}
	}
}

func ListVMs(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]

		list, err := clients.Dynamic.Resource(kube.VMGVR).Namespace(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			httpx.RespondWithAPIErrorMsg(w, err, "Failed to list VirtualMachines: "+err.Error())
			return
		}
		httpx.RespondWithJSON(w, http.StatusOK, list.Items)
	}
}
