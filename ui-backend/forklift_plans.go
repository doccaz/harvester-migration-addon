// forklift_plans.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/httpx"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// ListForkliftPlansHandler lists Forklift Plan CRs
func ListForkliftPlansHandler(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := clients.Dynamic.Resource(kube.ForkliftPlanGVR).Namespace("").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to list Forklift Plans: "+err.Error())
			return
		}
		httpx.RespondWithJSON(w, http.StatusOK, list.Items)
	}
}

// CreateForkliftPlanHandler creates NetworkMap, StorageMap, and Plan atomically
func CreateForkliftPlanHandler(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload CreateForkliftPlanPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			httpx.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
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

		_, err := clients.Dynamic.Resource(kube.ForkliftNetworkMapGVR).Namespace(payload.Namespace).Create(context.TODO(), networkMap, metav1.CreateOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to create Forklift NetworkMap: "+err.Error())
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

		_, err = clients.Dynamic.Resource(kube.ForkliftStorageMapGVR).Namespace(payload.Namespace).Create(context.TODO(), storageMap, metav1.CreateOptions{})
		if err != nil {
			// Cleanup NetworkMap
			if cleanupErr := clients.Dynamic.Resource(kube.ForkliftNetworkMapGVR).Namespace(payload.Namespace).Delete(context.TODO(), networkMapName, metav1.DeleteOptions{}); cleanupErr != nil {
				log.Warnf("Best-effort cleanup: failed to delete NetworkMap %s/%s: %v", payload.Namespace, networkMapName, cleanupErr)
			}
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to create Forklift StorageMap: "+err.Error())
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

		createdPlan, err := clients.Dynamic.Resource(kube.ForkliftPlanGVR).Namespace(payload.Namespace).Create(context.TODO(), plan, metav1.CreateOptions{})
		if err != nil {
			// Cleanup NetworkMap and StorageMap
			if cleanupErr := clients.Dynamic.Resource(kube.ForkliftNetworkMapGVR).Namespace(payload.Namespace).Delete(context.TODO(), networkMapName, metav1.DeleteOptions{}); cleanupErr != nil {
				log.Warnf("Best-effort cleanup: failed to delete NetworkMap %s/%s: %v", payload.Namespace, networkMapName, cleanupErr)
			}
			if cleanupErr := clients.Dynamic.Resource(kube.ForkliftStorageMapGVR).Namespace(payload.Namespace).Delete(context.TODO(), storageMapName, metav1.DeleteOptions{}); cleanupErr != nil {
				log.Warnf("Best-effort cleanup: failed to delete StorageMap %s/%s: %v", payload.Namespace, storageMapName, cleanupErr)
			}
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to create Forklift Plan: "+err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusCreated, createdPlan)
	}
}

// DeleteForkliftPlanHandler deletes a Forklift Plan and its associated NetworkMap/StorageMap
func DeleteForkliftPlanHandler(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		// Get the plan to find associated maps
		planObj, err := clients.Dynamic.Resource(kube.ForkliftPlanGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to get Forklift Plan: "+err.Error())
			return
		}

		networkMapName, _ := kube.NestedStringOrWarn(planObj.Object, "spec", "map", "network", "name")
		storageMapName, _ := kube.NestedStringOrWarn(planObj.Object, "spec", "map", "storage", "name")

		// Delete the Plan
		err = clients.Dynamic.Resource(kube.ForkliftPlanGVR).Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to delete Forklift Plan: "+err.Error())
			return
		}

		// Cleanup NetworkMap and StorageMap (best-effort)
		if networkMapName != "" {
			if delErr := clients.Dynamic.Resource(kube.ForkliftNetworkMapGVR).Namespace(namespace).Delete(context.TODO(), networkMapName, metav1.DeleteOptions{}); delErr != nil {
				log.Warnf("Failed to delete associated NetworkMap %s/%s: %v", namespace, networkMapName, delErr)
			}
		}
		if storageMapName != "" {
			if delErr := clients.Dynamic.Resource(kube.ForkliftStorageMapGVR).Namespace(namespace).Delete(context.TODO(), storageMapName, metav1.DeleteOptions{}); delErr != nil {
				log.Warnf("Failed to delete associated StorageMap %s/%s: %v", namespace, storageMapName, delErr)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// HandleGetForkliftPlanYAML returns the YAML representation of a Forklift Plan
func HandleGetForkliftPlanYAML(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		item, err := clients.Dynamic.Resource(kube.ForkliftPlanGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		yamlBytes, err := yaml.Marshal(item.Object)
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to marshal Forklift Plan to YAML: "+err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(yamlBytes); err != nil {
			log.Warnf("Failed to write response: %v", err)
		}
	}
}
