// forklift_handlers.go
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// CheckForkliftAvailability checks if the Forklift "host" Provider exists
func CheckForkliftAvailability(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		namespace := r.URL.Query().Get("namespace")
		if namespace == "" {
			namespace = "forklift"
		}

		// Check if the "host" provider exists
		_, err := clients.Dynamic.Resource(forkliftProviderGVR).Namespace(namespace).Get(context.TODO(), "host", metav1.GetOptions{})
		if err != nil {
			respondWithJSON(w, http.StatusOK, map[string]interface{}{
				"available":        false,
				"defaultNamespace": namespace,
				"message":          "Forklift host Provider not found in namespace " + namespace + ". Forklift features are unavailable.",
			})
			return
		}

		respondWithJSON(w, http.StatusOK, map[string]interface{}{
			"available":        true,
			"defaultNamespace": namespace,
		})
	}
}

// ListForkliftProvidersHandler lists Forklift Provider CRs (vsphere type only)
func ListForkliftProvidersHandler(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := clients.Dynamic.Resource(forkliftProviderGVR).Namespace("").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to list Forklift Providers: "+err.Error())
			return
		}

		// Optional type filter from query param
		typeFilter := r.URL.Query().Get("type")

		// Filter to source providers (vsphere + ova), exclude "host" and "openshift"
		var sourceProviders []unstructured.Unstructured
		for _, item := range list.Items {
			providerType, _ := getNestedStringOrWarn(item.Object, "spec", "type")
			if typeFilter != "" {
				// Exact type filter
				if providerType == typeFilter {
					sourceProviders = append(sourceProviders, item)
				}
			} else {
				// Include all source providers
				if providerType == "vsphere" || providerType == "ova" {
					sourceProviders = append(sourceProviders, item)
				}
			}
		}
		respondWithJSON(w, http.StatusOK, sourceProviders)
	}
}

// CreateForkliftProviderHandler creates a Forklift Provider with its associated Secret
func CreateForkliftProviderHandler(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload CreateForkliftProviderPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			respondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		if payload.Namespace == "" {
			payload.Namespace = "forklift"
		}

		// Default provider type to vsphere
		providerType := payload.ProviderType
		if providerType == "" {
			providerType = "vsphere"
		}

		// 1. Create the Opaque Secret with Forklift's expected format
		secretName := payload.Name + "-secret"
		var secretData map[string]string
		if providerType == "ova" {
			// OVA providers only need the NFS URL
			secretData = map[string]string{
				"url": payload.URL,
			}
		} else {
			// vSphere providers need credentials
			insecureSkipVerify := "true"
			if payload.InsecureSkipVerify != nil && !*payload.InsecureSkipVerify {
				insecureSkipVerify = "false"
			}
			secretData = map[string]string{
				"user":               payload.Username,
				"password":           payload.Password,
				"url":                payload.URL,
				"insecureSkipVerify": insecureSkipVerify,
			}
			if insecureSkipVerify == "false" && payload.CACert != "" {
				secretData["cacert"] = payload.CACert
			}
		}

		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: payload.Namespace,
				Labels: map[string]string{
					"createdForProviderType": providerType,
					"createdForResourceType": "providers",
				},
			},
			Type:       v1.SecretTypeOpaque,
			StringData: secretData,
		}
		_, err := clients.Clientset.CoreV1().Secrets(payload.Namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to create Forklift secret: "+err.Error())
			return
		}

		// 2. Create the Forklift Provider CR
		providerSpec := map[string]interface{}{
			"type": providerType,
			"url":  payload.URL,
			"secret": map[string]interface{}{
				"name":      secretName,
				"namespace": payload.Namespace,
			},
		}

		// vSphere providers need sdkEndpoint settings; OVA providers have no settings
		if providerType == "vsphere" {
			settings := map[string]interface{}{
				"sdkEndpoint": func() string {
					if payload.SdkEndpoint == "esxi" {
						return "esxi"
					}
					return "vcenter"
				}(),
			}
			if payload.VddkInitImage != "" {
				settings["vddkInitImage"] = payload.VddkInitImage
			}
			providerSpec["settings"] = settings
		}

		providerAnnotations := map[string]interface{}{}
		if payload.VddkInitImage == "" {
			providerAnnotations["forklift.konveyor.io/empty-vddk-init-image"] = "yes"
		}

		provider := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "forklift.konveyor.io/v1beta1",
				"kind":       "Provider",
				"metadata": map[string]interface{}{
					"name":        payload.Name,
					"namespace":   payload.Namespace,
					"annotations": providerAnnotations,
				},
				"spec": providerSpec,
			},
		}

		createdObj, err := clients.Dynamic.Resource(forkliftProviderGVR).Namespace(payload.Namespace).Create(context.TODO(), provider, metav1.CreateOptions{})
		if err != nil {
			// Clean up secret on failure
			if cleanupErr := clients.Clientset.CoreV1().Secrets(payload.Namespace).Delete(context.TODO(), secretName, metav1.DeleteOptions{}); cleanupErr != nil {
				log.Warnf("Best-effort cleanup: failed to delete secret %s/%s: %v", payload.Namespace, secretName, cleanupErr)
			}
			respondWithError(w, http.StatusInternalServerError, "Failed to create Forklift Provider: "+err.Error())
			return
		}

		respondWithJSON(w, http.StatusCreated, createdObj)
	}
}

// GetForkliftProviderDetails returns a single Forklift Provider with its secret info
func GetForkliftProviderDetails(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		providerObj, err := clients.Dynamic.Resource(forkliftProviderGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			respondWithError(w, http.StatusNotFound, "Failed to get Forklift Provider: "+err.Error())
			return
		}

		// Enrich with info from secret
		secretName, _ := getNestedStringOrWarn(providerObj.Object, "spec", "secret", "name")
		if secretName != "" {
			secret, err := clients.Clientset.CoreV1().Secrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
			if err == nil {
				specMap := providerObj.Object["spec"].(map[string]interface{})
				specMap["username"] = string(secret.Data["user"])
				specMap["insecureSkipVerify"] = string(secret.Data["insecureSkipVerify"])
				specMap["hasCACert"] = len(secret.Data["cacert"]) > 0
			}
		}

		respondWithJSON(w, http.StatusOK, providerObj)
	}
}

// UpdateForkliftProviderHandler updates a Forklift Provider and its Secret
func UpdateForkliftProviderHandler(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		var payload CreateForkliftProviderPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			respondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		providerObj, err := clients.Dynamic.Resource(forkliftProviderGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			respondWithError(w, http.StatusNotFound, "Failed to get Forklift Provider: "+err.Error())
			return
		}

		// Update secret if credentials or TLS settings provided
		secretName, found := getNestedStringOrWarn(providerObj.Object, "spec", "secret", "name")
		if !found {
			respondWithError(w, http.StatusInternalServerError, "Forklift Provider missing secret name")
			return
		}
		needsSecretUpdate := payload.Username != "" || payload.Password != "" ||
			payload.URL != "" || payload.InsecureSkipVerify != nil || payload.CACert != ""
		if needsSecretUpdate {
			secret, err := clients.Clientset.CoreV1().Secrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
			if err != nil {
				respondWithError(w, http.StatusInternalServerError, "Failed to get associated secret: "+err.Error())
				return
			}

			if secret.StringData == nil {
				secret.StringData = make(map[string]string)
			}
			if payload.Username != "" {
				secret.StringData["user"] = payload.Username
			}
			if payload.Password != "" {
				secret.StringData["password"] = payload.Password
			}
			if payload.URL != "" {
				secret.StringData["url"] = payload.URL
			}
			if payload.InsecureSkipVerify != nil {
				if *payload.InsecureSkipVerify {
					secret.StringData["insecureSkipVerify"] = "true"
					delete(secret.Data, "cacert")
				} else {
					secret.StringData["insecureSkipVerify"] = "false"
					if payload.CACert != "" {
						secret.StringData["cacert"] = payload.CACert
					}
				}
			}
			_, err = clients.Clientset.CoreV1().Secrets(namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
			if err != nil {
				respondWithError(w, http.StatusInternalServerError, "Failed to update secret: "+err.Error())
				return
			}
		}

		// Update the Provider URL
		if payload.URL != "" {
			if err := unstructured.SetNestedField(providerObj.Object, payload.URL, "spec", "url"); err != nil {
				respondWithError(w, http.StatusInternalServerError, "Failed to set URL: "+err.Error())
				return
			}
		}

		// Update sdkEndpoint setting
		if payload.SdkEndpoint != "" {
			if err := unstructured.SetNestedField(providerObj.Object, payload.SdkEndpoint, "spec", "settings", "sdkEndpoint"); err != nil {
				respondWithError(w, http.StatusInternalServerError, "Failed to set sdkEndpoint: "+err.Error())
				return
			}
		}

		// Update VDDK init image
		if payload.VddkInitImage != "" {
			if err := unstructured.SetNestedField(providerObj.Object, payload.VddkInitImage, "spec", "settings", "vddkInitImage"); err != nil {
				respondWithError(w, http.StatusInternalServerError, "Failed to set vddkInitImage: "+err.Error())
				return
			}
			// Remove the empty-vddk annotation since we now have an image
			annotations, _, _ := unstructured.NestedStringMap(providerObj.Object, "metadata", "annotations")
			if annotations != nil {
				delete(annotations, "forklift.konveyor.io/empty-vddk-init-image")
				if err := unstructured.SetNestedStringMap(providerObj.Object, annotations, "metadata", "annotations"); err != nil {
					log.Warnf("Failed to update annotations: %v", err)
				}
			}
		}

		updatedObj, err := clients.Dynamic.Resource(forkliftProviderGVR).Namespace(namespace).Update(context.TODO(), providerObj, metav1.UpdateOptions{})
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to update Forklift Provider: "+err.Error())
			return
		}

		respondWithJSON(w, http.StatusOK, updatedObj)
	}
}

// DeleteForkliftProviderHandler deletes a Forklift Provider and its associated Secret
func DeleteForkliftProviderHandler(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		providerObj, err := clients.Dynamic.Resource(forkliftProviderGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to get Forklift Provider: "+err.Error())
			return
		}
		secretName, _ := getNestedStringOrWarn(providerObj.Object, "spec", "secret", "name")

		err = clients.Dynamic.Resource(forkliftProviderGVR).Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to delete Forklift Provider: "+err.Error())
			return
		}

		if secretName != "" {
			err = clients.Clientset.CoreV1().Secrets(namespace).Delete(context.TODO(), secretName, metav1.DeleteOptions{})
			if err != nil {
				log.Warnf("Failed to delete associated Forklift secret %s/%s: %v", namespace, secretName, err)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// HandleGetForkliftInventory fetches vCenter inventory using Forklift Provider credentials
func HandleGetForkliftInventory(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		log.Infof("Fetching inventory for Forklift Provider %s/%s", namespace, name)

		providerObj, err := clients.Dynamic.Resource(forkliftProviderGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to get Forklift Provider: "+err.Error())
			return
		}

		providerURL, found := getNestedStringOrWarn(providerObj.Object, "spec", "url")
		if !found {
			respondWithError(w, http.StatusInternalServerError, "Forklift Provider missing URL")
			return
		}
		secretName, found := getNestedStringOrWarn(providerObj.Object, "spec", "secret", "name")
		if !found {
			respondWithError(w, http.StatusInternalServerError, "Forklift Provider missing secret name")
			return
		}
		secretNamespace, found := getNestedStringOrWarn(providerObj.Object, "spec", "secret", "namespace")
		if !found {
			respondWithError(w, http.StatusInternalServerError, "Forklift Provider missing secret namespace")
			return
		}

		secret, err := clients.Clientset.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to get Forklift credentials secret: "+err.Error())
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
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		respondWithJSON(w, http.StatusOK, inventory)
	}
}

// ListForkliftPlansHandler lists Forklift Plan CRs
func ListForkliftPlansHandler(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := clients.Dynamic.Resource(forkliftPlanGVR).Namespace("").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to list Forklift Plans: "+err.Error())
			return
		}
		respondWithJSON(w, http.StatusOK, list.Items)
	}
}

// CreateForkliftPlanHandler creates NetworkMap, StorageMap, and Plan atomically
func CreateForkliftPlanHandler(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload CreateForkliftPlanPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			respondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		if payload.Namespace == "" {
			payload.Namespace = "forklift"
		}

		// Determine the namespace where Forklift's "host" provider lives
		hostProviderNs := payload.HostProviderNamespace
		if hostProviderNs == "" {
			hostProviderNs = "forklift"
		}

		log.Infof("Creating Forklift migration plan: %s in namespace %s", payload.Name, payload.Namespace)

		// 1. Create NetworkMap
		networkMapName := payload.Name + "-network-map"
		networkMapEntries := make([]interface{}, len(payload.NetworkMappings))
		for i, nm := range payload.NetworkMappings {
			dest := map[string]interface{}{
				"type": nm.DestinationType,
			}
			if nm.DestinationType == "multus" && nm.DestinationName != "" {
				dest["name"] = nm.DestinationName
				dest["namespace"] = nm.DestinationNamespace
			}
			source := map[string]interface{}{
				"id": nm.SourceID,
			}
			if nm.SourceName != "" {
				source["name"] = nm.SourceName
			}
			networkMapEntries[i] = map[string]interface{}{
				"source":      source,
				"destination": dest,
			}
		}

		networkMap := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "forklift.konveyor.io/v1beta1",
				"kind":       "NetworkMap",
				"metadata": map[string]interface{}{
					"name":      networkMapName,
					"namespace": payload.Namespace,
				},
				"spec": map[string]interface{}{
					"map": networkMapEntries,
					"provider": map[string]interface{}{
						"source": map[string]interface{}{
							"apiVersion": "forklift.konveyor.io/v1beta1",
							"kind":       "Provider",
							"name":       payload.ProviderName,
							"namespace":  payload.ProviderNamespace,
						},
						"destination": map[string]interface{}{
							"apiVersion": "forklift.konveyor.io/v1beta1",
							"kind":       "Provider",
							"name":       "host",
							"namespace":  hostProviderNs,
						},
					},
				},
			},
		}

		_, err := clients.Dynamic.Resource(forkliftNetworkMapGVR).Namespace(payload.Namespace).Create(context.TODO(), networkMap, metav1.CreateOptions{})
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to create Forklift NetworkMap: "+err.Error())
			return
		}

		// Determine provider type for plan creation logic
		providerType := payload.ProviderType
		if providerType == "" {
			providerType = "vsphere"
		}

		// 2. Create StorageMap
		storageMapName := payload.Name + "-storage-map"
		storageMapEntries := make([]interface{}, len(payload.StorageMappings))
		for i, sm := range payload.StorageMappings {
			dest := map[string]interface{}{
				"storageClass": sm.DestinationStorageClass,
			}
			if sm.VolumeMode != "" {
				dest["volumeMode"] = sm.VolumeMode
			}
			if sm.AccessMode != "" {
				dest["accessMode"] = sm.AccessMode
			}
			// OVA providers use source.name (disk filename), vSphere uses source.id (moRef)
			source := map[string]interface{}{}
			if providerType == "ova" && sm.SourceName != "" {
				source["name"] = sm.SourceName
			} else {
				source["id"] = sm.SourceID
			}
			storageMapEntries[i] = map[string]interface{}{
				"source":      source,
				"destination": dest,
			}
		}

		storageMap := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "forklift.konveyor.io/v1beta1",
				"kind":       "StorageMap",
				"metadata": map[string]interface{}{
					"name":      storageMapName,
					"namespace": payload.Namespace,
				},
				"spec": map[string]interface{}{
					"map": storageMapEntries,
					"provider": map[string]interface{}{
						"source": map[string]interface{}{
							"apiVersion": "forklift.konveyor.io/v1beta1",
							"kind":       "Provider",
							"name":       payload.ProviderName,
							"namespace":  payload.ProviderNamespace,
						},
						"destination": map[string]interface{}{
							"apiVersion": "forklift.konveyor.io/v1beta1",
							"kind":       "Provider",
							"name":       "host",
							"namespace":  hostProviderNs,
						},
					},
				},
			},
		}

		_, err = clients.Dynamic.Resource(forkliftStorageMapGVR).Namespace(payload.Namespace).Create(context.TODO(), storageMap, metav1.CreateOptions{})
		if err != nil {
			// Cleanup NetworkMap
			if cleanupErr := clients.Dynamic.Resource(forkliftNetworkMapGVR).Namespace(payload.Namespace).Delete(context.TODO(), networkMapName, metav1.DeleteOptions{}); cleanupErr != nil {
				log.Warnf("Best-effort cleanup: failed to delete NetworkMap %s/%s: %v", payload.Namespace, networkMapName, cleanupErr)
			}
			respondWithError(w, http.StatusInternalServerError, "Failed to create Forklift StorageMap: "+err.Error())
			return
		}

		// 3. Create Plan
		vmEntries := make([]interface{}, len(payload.VMs))
		for i, vm := range payload.VMs {
			entry := map[string]interface{}{
				"id":   vm.ID,
				"name": vm.Name,
			}
			if vm.TargetName != "" {
				entry["targetName"] = vm.TargetName
			}
			vmEntries[i] = entry
		}

		planAnnotations := map[string]interface{}{}
		if payload.PopulatorLabels {
			planAnnotations["populatorLabels"] = "True"
		}
		// Store source VM characteristics as annotations (same pattern as VMIC)
		if payload.SourceVmCpu > 0 {
			planAnnotations["migration.harvesterhci.io/original-cpu"] = fmt.Sprintf("%d", payload.SourceVmCpu)
		}
		if payload.SourceVmMemoryMB > 0 {
			planAnnotations["migration.harvesterhci.io/original-memory-mb"] = fmt.Sprintf("%d", payload.SourceVmMemoryMB)
		}
		if payload.SourceVmDiskSizeGB > 0 {
			planAnnotations["migration.harvesterhci.io/original-disk-size-gb"] = fmt.Sprintf("%d", payload.SourceVmDiskSizeGB)
		}
		if payload.SourceVmDisks != "" {
			planAnnotations["migration.harvesterhci.io/original-disks"] = payload.SourceVmDisks
		}
		if payload.SourceVmNetworks != "" {
			planAnnotations["migration.harvesterhci.io/original-networks"] = payload.SourceVmNetworks
		}
		if payload.DefaultNetworkInterfaceModel != "" {
			planAnnotations["migration.harvesterhci.io/default-nic-model"] = payload.DefaultNetworkInterfaceModel
		}

		plan := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "forklift.konveyor.io/v1beta1",
				"kind":       "Plan",
				"metadata": map[string]interface{}{
					"name":        payload.Name,
					"namespace":   payload.Namespace,
					"annotations": planAnnotations,
				},
				"spec": map[string]interface{}{
					"map": map[string]interface{}{
						"network": map[string]interface{}{
							"apiVersion": "forklift.konveyor.io/v1beta1",
							"kind":       "NetworkMap",
							"name":       networkMapName,
							"namespace":  payload.Namespace,
						},
						"storage": map[string]interface{}{
							"apiVersion": "forklift.konveyor.io/v1beta1",
							"kind":       "StorageMap",
							"name":       storageMapName,
							"namespace":  payload.Namespace,
						},
					},
					"provider": map[string]interface{}{
						"source": map[string]interface{}{
							"apiVersion": "forklift.konveyor.io/v1beta1",
							"kind":       "Provider",
							"name":       payload.ProviderName,
							"namespace":  payload.ProviderNamespace,
						},
						"destination": map[string]interface{}{
							"apiVersion": "forklift.konveyor.io/v1beta1",
							"kind":       "Provider",
							"name":       "host",
							"namespace":  hostProviderNs,
						},
					},
					"targetNamespace": payload.TargetNamespace,
					"warm": func() bool {
						if providerType == "ova" {
							return false
						}
						return payload.Warm
					}(),
					"migrateSharedDisks":      payload.MigrateSharedDisks,
					"preserveClusterCpuModel": payload.PreserveClusterCpuModel,
					"preserveStaticIPs":       payload.PreserveStaticIPs,
					"vms":                     vmEntries,
				},
			},
		}

		createdPlan, err := clients.Dynamic.Resource(forkliftPlanGVR).Namespace(payload.Namespace).Create(context.TODO(), plan, metav1.CreateOptions{})
		if err != nil {
			// Cleanup NetworkMap and StorageMap
			if cleanupErr := clients.Dynamic.Resource(forkliftNetworkMapGVR).Namespace(payload.Namespace).Delete(context.TODO(), networkMapName, metav1.DeleteOptions{}); cleanupErr != nil {
				log.Warnf("Best-effort cleanup: failed to delete NetworkMap %s/%s: %v", payload.Namespace, networkMapName, cleanupErr)
			}
			if cleanupErr := clients.Dynamic.Resource(forkliftStorageMapGVR).Namespace(payload.Namespace).Delete(context.TODO(), storageMapName, metav1.DeleteOptions{}); cleanupErr != nil {
				log.Warnf("Best-effort cleanup: failed to delete StorageMap %s/%s: %v", payload.Namespace, storageMapName, cleanupErr)
			}
			respondWithError(w, http.StatusInternalServerError, "Failed to create Forklift Plan: "+err.Error())
			return
		}

		respondWithJSON(w, http.StatusCreated, createdPlan)
	}
}

// DeleteForkliftPlanHandler deletes a Forklift Plan and its associated NetworkMap/StorageMap
func DeleteForkliftPlanHandler(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		// Get the plan to find associated maps
		planObj, err := clients.Dynamic.Resource(forkliftPlanGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to get Forklift Plan: "+err.Error())
			return
		}

		networkMapName, _ := getNestedStringOrWarn(planObj.Object, "spec", "map", "network", "name")
		storageMapName, _ := getNestedStringOrWarn(planObj.Object, "spec", "map", "storage", "name")

		// Delete the Plan
		err = clients.Dynamic.Resource(forkliftPlanGVR).Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to delete Forklift Plan: "+err.Error())
			return
		}

		// Cleanup NetworkMap and StorageMap (best-effort)
		if networkMapName != "" {
			if delErr := clients.Dynamic.Resource(forkliftNetworkMapGVR).Namespace(namespace).Delete(context.TODO(), networkMapName, metav1.DeleteOptions{}); delErr != nil {
				log.Warnf("Failed to delete associated NetworkMap %s/%s: %v", namespace, networkMapName, delErr)
			}
		}
		if storageMapName != "" {
			if delErr := clients.Dynamic.Resource(forkliftStorageMapGVR).Namespace(namespace).Delete(context.TODO(), storageMapName, metav1.DeleteOptions{}); delErr != nil {
				log.Warnf("Failed to delete associated StorageMap %s/%s: %v", namespace, storageMapName, delErr)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// HandleGetForkliftPlanYAML returns the YAML representation of a Forklift Plan
func HandleGetForkliftPlanYAML(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		item, err := clients.Dynamic.Resource(forkliftPlanGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		yamlBytes, err := yaml.Marshal(item.Object)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to marshal Forklift Plan to YAML: "+err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(yamlBytes); err != nil {
			log.Warnf("Failed to write response: %v", err)
		}
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
func HandleGetForkliftOvaInventory(clients *K8sClients) http.HandlerFunc {
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
			respondWithError(w, http.StatusBadRequest, "Unsupported OVA inventory resource: "+resource)
			return
		}

		// 1. Get the Provider CR to obtain its UID
		providerObj, err := clients.Dynamic.Resource(forkliftProviderGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			respondWithError(w, http.StatusNotFound, "Failed to get OVA provider: "+err.Error())
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
				respondWithError(w, http.StatusInternalServerError, "Cannot find forklift-inventory service: "+err.Error())
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
			respondWithError(w, http.StatusInternalServerError, "Failed to build inventory request: "+err.Error())
			return
		}
		resp, err := inventoryClient.Do(req)
		if err != nil {
			respondWithError(w, http.StatusBadGateway, "Failed to reach forklift-inventory: "+err.Error())
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

// HandleGetForkliftLogs fetches logs from Forklift controller pods and migration worker pods.
// Forklift labels worker pods with "plan-name" = <planName> and "forklift.app" = virt-v2v | consumer | virt-v2v-inspection.
// Worker pods run in the plan's targetNamespace; hooks run in the plan namespace.
// Controller pods use structured JSON logging with "plan", "migration", "vm" fields.
func HandleGetForkliftLogs(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		planNamespace := vars["namespace"]
		planName := vars["name"]

		forkliftNs := r.URL.Query().Get("forkliftNamespace")
		if forkliftNs == "" {
			forkliftNs = planNamespace
		}
		showAll := r.URL.Query().Get("all") == "true"
		errorsOnly := r.URL.Query().Get("errors") == "true"

		log.Debugf("Fetching Forklift logs for plan %s/%s (forklift ns: %s)", planNamespace, planName, forkliftNs)

		// Get the plan to find target namespace and VM IDs/names
		var targetNamespace string
		var vmIDs []string
		planObj, err := clients.Dynamic.Resource(forkliftPlanGVR).Namespace(planNamespace).Get(context.TODO(), planName, metav1.GetOptions{})
		if err == nil {
			targetNamespace, _ = getNestedStringOrWarn(planObj.Object, "spec", "targetNamespace")
			vms, _, vmsErr := unstructured.NestedSlice(planObj.Object, "spec", "vms")
			if vmsErr != nil {
				log.Warnf("Error reading spec.vms: %v", vmsErr)
			}
			for _, vm := range vms {
				if vmMap, ok := vm.(map[string]interface{}); ok {
					if id, ok := vmMap["id"].(string); ok && id != "" {
						vmIDs = append(vmIDs, id)
					}
				}
			}
		}

		// Related resource names that appear in controller logs
		migrationName := planName + "-migration"
		networkMapName := planName + "-network-map"
		storageMapName := planName + "-storage-map"
		controllerMatchTerms := []string{planName, migrationName, networkMapName, storageMapName}

		var logOutput strings.Builder

		isErrorLine := func(line string) bool {
			l := strings.ToLower(line)
			return strings.Contains(l, "error") || strings.Contains(l, "fail") ||
				strings.Contains(l, "warn") || strings.Contains(l, "critical") ||
				strings.Contains(l, "\"level\":\"error\"") || strings.Contains(l, "\"level\":\"warn\"")
		}

		// fetchAndWriteLogs streams logs from a pod/container, optionally filtering.
		// If filterTerms is nil, all lines are included (worker pods are already plan-specific).
		fetchAndWriteLogs := func(ns string, pod v1.Pod, filterTerms []string, header string) {
			for _, cs := range append(pod.Spec.InitContainers, pod.Spec.Containers...) {
				req := clients.Clientset.CoreV1().Pods(ns).GetLogs(pod.Name, &v1.PodLogOptions{
					Container: cs.Name,
				})
				stream, err := req.Stream(context.TODO())
				if err != nil {
					// Skip containers that can't be read (not started, etc.)
					continue
				}

				headerWritten := false
				scanner := bufio.NewScanner(stream)
				buf := make([]byte, 0, 64*1024)
				scanner.Buffer(buf, 1024*1024)

				for scanner.Scan() {
					line := scanner.Text()
					include := showAll

					if !include && filterTerms != nil {
						// Controller pod: only include lines mentioning our plan/resources
						for _, term := range filterTerms {
							if strings.Contains(line, term) {
								include = true
								break
							}
						}
					} else if filterTerms == nil {
						// Worker pod: include everything
						include = true
					}

					if include && errorsOnly && !showAll {
						include = isErrorLine(line)
					}

					if include {
						if !headerWritten && header != "" {
							logOutput.WriteString(header)
							headerWritten = true
						}
						logOutput.WriteString(line + "\n")
					}
				}
				stream.Close()
			}
		}

		// ── 1. Forklift controller pod (filtered by plan name) ──
		// The controller pod is labeled app=forklift-controller in the forklift namespace
		controllerPods, _ := clients.Clientset.CoreV1().Pods(forkliftNs).List(context.TODO(), metav1.ListOptions{
			LabelSelector: "app=forklift-controller",
		})
		if controllerPods == nil || len(controllerPods.Items) == 0 {
			// Fallback: any pod with "forklift-controller" in the name
			allPods, _ := clients.Clientset.CoreV1().Pods(forkliftNs).List(context.TODO(), metav1.ListOptions{})
			if allPods != nil {
				for _, p := range allPods.Items {
					if strings.Contains(p.Name, "forklift-controller") {
						if controllerPods == nil {
							controllerPods = &v1.PodList{}
						}
						controllerPods.Items = append(controllerPods.Items, p)
					}
				}
			}
		}
		if controllerPods != nil {
			for _, pod := range controllerPods.Items {
				fetchAndWriteLogs(forkliftNs, pod, controllerMatchTerms,
					fmt.Sprintf("\n=== Forklift Controller: %s ===\n", pod.Name))
			}
		}

		// ── 2. Migration worker pods (label: plan-name=<planName>) ──
		// These run in targetNamespace and include virt-v2v, virt-v2v-inspection, consumer pods.
		// Forklift labels them with plan-name=<planName>.
		searchNamespaces := []string{}
		if targetNamespace != "" {
			searchNamespaces = append(searchNamespaces, targetNamespace)
		}
		if planNamespace != targetNamespace {
			searchNamespaces = append(searchNamespaces, planNamespace)
		}

		for _, ns := range searchNamespaces {
			// Direct label query — most efficient
			workerPods, err := clients.Clientset.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{
				LabelSelector: "plan-name=" + planName,
			})
			if err == nil {
				for _, pod := range workerPods.Items {
					appLabel := pod.Labels["forklift.app"]
					podType := "Worker"
					switch appLabel {
					case "virt-v2v":
						podType = "virt-v2v Conversion"
					case "virt-v2v-inspection":
						podType = "virt-v2v Inspection"
					case "consumer":
						podType = "Consumer"
					}
					vmID := pod.Labels["vmID"]
					vmInfo := ""
					if vmID != "" {
						vmInfo = fmt.Sprintf(" (VM: %s)", vmID)
					}
					fetchAndWriteLogs(ns, pod, nil,
						fmt.Sprintf("\n=== %s: %s/%s%s ===\n", podType, ns, pod.Name, vmInfo))
				}
			}

			// Also look for populator pods (created by CDI, name prefix "populate-")
			// and pods whose name starts with the plan name (hook jobs, converter jobs)
			allPods, err := clients.Clientset.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{})
			if err == nil {
				seen := map[string]bool{}
				if workerPods != nil {
					for _, p := range workerPods.Items {
						seen[p.Name] = true
					}
				}
				for _, pod := range allPods.Items {
					if seen[pod.Name] {
						continue
					}
					isRelevant := false
					podType := "Related Pod"

					// Check if pod name starts with planName (hook jobs, converter jobs)
					if strings.HasPrefix(pod.Name, planName+"-") {
						isRelevant = true
						podType = "Plan Pod"
					}

					// Check for populator pods by looking at migration label
					if !isRelevant && strings.HasPrefix(pod.Name, "populate-") {
						if _, hasMigLabel := pod.Labels["migration"]; hasMigLabel {
							// Check if this populator's migration label matches our plan's migration
							isRelevant = true
							podType = "CDI Populator"
						}
					}

					// Check for converter jobs
					if !isRelevant && strings.HasPrefix(pod.Name, "convert-") {
						for _, vmID := range vmIDs {
							if strings.Contains(pod.Name, vmID) {
								isRelevant = true
								podType = "Disk Converter"
								break
							}
						}
					}

					if isRelevant {
						fetchAndWriteLogs(ns, pod, nil,
							fmt.Sprintf("\n=== %s: %s/%s ===\n", podType, ns, pod.Name))
					}
				}
			}
		}

		if logOutput.Len() == 0 {
			fmt.Fprintf(&logOutput, "No matching log entries found for plan '%s'.\n\n", planName)
			logOutput.WriteString("Troubleshooting tips:\n")
			logOutput.WriteString("  1. Disable 'Only relevant' to see all Forklift controller logs\n")
			if targetNamespace != "" {
				fmt.Fprintf(&logOutput, "  2. Check worker pods: kubectl get pods -n %s -l plan-name=%s\n", targetNamespace, planName)
				fmt.Fprintf(&logOutput, "  3. Check populator pods: kubectl get pods -n %s | grep populate-\n", targetNamespace)
			}
			fmt.Fprintf(&logOutput, "  4. Controller logs: kubectl logs -n %s -l app=forklift-controller | grep %s\n", forkliftNs, planName)
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(logOutput.String())); err != nil {
			log.Warnf("Failed to write response: %v", err)
		}
	}
}

// CreateForkliftMigrationHandler creates a Migration CR to start executing a Forklift Plan
func CreateForkliftMigrationHandler(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		log.Infof("Creating Migration for Forklift Plan %s/%s", namespace, name)

		migration := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "forklift.konveyor.io/v1beta1",
				"kind":       "Migration",
				"metadata": map[string]interface{}{
					"name":      name + "-migration",
					"namespace": namespace,
				},
				"spec": map[string]interface{}{
					"plan": map[string]interface{}{
						"name":      name,
						"namespace": namespace,
					},
				},
			},
		}

		createdObj, err := clients.Dynamic.Resource(forkliftMigrationGVR).Namespace(namespace).Create(context.TODO(), migration, metav1.CreateOptions{})
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to create Forklift Migration: "+err.Error())
			return
		}

		respondWithJSON(w, http.StatusCreated, createdObj)
	}
}

// DeleteForkliftMigrationHandler deletes an existing Migration CR for a Plan
func DeleteForkliftMigrationHandler(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		migrationName := name + "-migration"
		log.Infof("Deleting Forklift Migration %s/%s", namespace, migrationName)

		err := clients.Dynamic.Resource(forkliftMigrationGVR).Namespace(namespace).Delete(context.TODO(), migrationName, metav1.DeleteOptions{})
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to delete Forklift Migration: "+err.Error())
			return
		}

		respondWithJSON(w, http.StatusOK, map[string]string{"message": "Migration deleted"})
	}
}

// GetForkliftMigrationStatus returns the status of Migrations for a Plan
func GetForkliftMigrationStatus(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		// List all migrations in the namespace
		list, err := clients.Dynamic.Resource(forkliftMigrationGVR).Namespace(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to list Forklift Migrations: "+err.Error())
			return
		}

		// Find the migration for this plan (prefer the most recent one)
		var latestMigration map[string]interface{}
		var latestTime string
		for _, item := range list.Items {
			planName, _ := getNestedStringOrWarn(item.Object, "spec", "plan", "name")
			if planName == name {
				created := item.GetCreationTimestamp().Format("2006-01-02T15:04:05Z")
				if created > latestTime {
					latestTime = created
					obj := item.Object
					latestMigration = obj
				}
			}
		}
		if latestMigration != nil {
			respondWithJSON(w, http.StatusOK, latestMigration)
			return
		}

		respondWithJSON(w, http.StatusOK, map[string]interface{}{"message": "No migration found for this plan"})
	}
}
