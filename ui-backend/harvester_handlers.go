// harvester_handlers.go
package main

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/httpx"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

func ListNamespacesHandler(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		namespaces, err := clients.Clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.RespondWithJSON(w, http.StatusOK, namespaces.Items)
	}
}

func CreateNamespaceHandler(clients *K8sClients) http.HandlerFunc {
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
			httpx.RespondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusCreated, map[string]string{"status": "namespace created"})
	}
}

func ListVlanConfigsHandler(clients *K8sClients) http.HandlerFunc {
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
			httpx.RespondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		log.Debugf("Fetched VLAN definitions: %+v", list.Items)
		httpx.RespondWithJSON(w, http.StatusOK, list.Items)
	}
}

func ListStorageClassesHandler(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scs, err := clients.Clientset.StorageV1().StorageClasses().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.RespondWithJSON(w, http.StatusOK, scs.Items)
	}
}

// HandleGetResource returns a namespaced resource as JSON
func HandleGetResource(clients *K8sClients, gvr schema.GroupVersionResource) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		item, err := clients.Dynamic.Resource(gvr).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusNotFound, err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusOK, item.Object)
	}
}

func HandleGetSourceYAML(clients *K8sClients, gvr schema.GroupVersionResource) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		log.Infof("Fetching YAML for source %s/%s", namespace, name)

		item, err := clients.Dynamic.Resource(gvr).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		yamlBytes, err := yaml.Marshal(item.Object)
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to marshal source to YAML: "+err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(yamlBytes); err != nil {
			log.Warnf("Failed to write response: %v", err)
		}
	}
}

func ListVMsHandler(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]

		list, err := clients.Dynamic.Resource(vmGVR).Namespace(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to list VirtualMachines: "+err.Error())
			return
		}
		httpx.RespondWithJSON(w, http.StatusOK, list.Items)
	}
}

// --- OvaSource Handlers ---
