// plans.go
package vmic

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/httpx"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

func CreatePlan(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var plan VirtualMachineImport
		if err := json.NewDecoder(r.Body).Decode(&plan); err != nil {
			httpx.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		log.Infof("Creating VirtualMachineImport CR: %s in namespace %s", plan.ObjectMeta.Name, plan.ObjectMeta.Namespace)
		log.Debugf("Received plan payload: %+v", plan)

		unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&plan)
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to convert plan to unstructured object: "+err.Error())
			return
		}

		createdObj, err := clients.Dynamic.Resource(kube.VMIGVR).Namespace(plan.ObjectMeta.Namespace).Create(context.TODO(), &unstructured.Unstructured{Object: unstructuredObj}, metav1.CreateOptions{})
		if err != nil {
			log.Errorf("Failed to create VirtualMachineImport CR: %v", err)
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to create VirtualMachineImport CR: "+err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusCreated, createdObj)
	}
}

func ListPlans(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := clients.Dynamic.Resource(kube.VMIGVR).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to list VirtualMachineImport CRs: "+err.Error())
			return
		}
		httpx.RespondWithJSON(w, http.StatusOK, list.Items)
	}
}

func DeletePlan(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		log.Infof("Deleting VirtualMachineImport CR: %s in namespace %s", name, namespace)
		err := clients.Dynamic.Resource(kube.VMIGVR).Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func UpdatePlan(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		var payload UpdatePlanPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			httpx.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		item, err := clients.Dynamic.Resource(kube.VMIGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusNotFound, "Plan not found: "+err.Error())
			return
		}

		// setOrClearString sets a spec field to a non-empty value or removes it when empty.
		setOrClearString := func(val *string, path ...string) {
			if val == nil {
				return
			}
			if *val == "" {
				unstructured.RemoveNestedField(item.Object, path...)
			} else {
				kube.SetNested(item.Object, *val, path...)
			}
		}

		setOrClearString(payload.VirtualMachineName, "spec", "virtualMachineName")
		setOrClearString(payload.StorageClass, "spec", "storageClass")
		setOrClearString(payload.Folder, "spec", "folder")
		setOrClearString(payload.DefaultNetworkInterfaceModel, "spec", "defaultNetworkInterfaceModel")
		setOrClearString(payload.DefaultDiskBusType, "spec", "defaultDiskBusType")

		if payload.ForcePowerOff != nil {
			kube.SetNested(item.Object, *payload.ForcePowerOff, "spec", "forcePowerOff")
		}
		if payload.SkipPreflightChecks != nil {
			kube.SetNested(item.Object, *payload.SkipPreflightChecks, "spec", "skipPreflightChecks")
		}
		if payload.GracefulShutdownTimeoutSeconds != nil {
			if *payload.GracefulShutdownTimeoutSeconds == 0 {
				unstructured.RemoveNestedField(item.Object, "spec", "gracefulShutdownTimeoutSeconds")
			} else {
				kube.SetNested(item.Object, *payload.GracefulShutdownTimeoutSeconds, "spec", "gracefulShutdownTimeoutSeconds")
			}
		}
		if payload.NetworkMapping != nil {
			mappings := make([]interface{}, 0, len(*payload.NetworkMapping))
			for _, m := range *payload.NetworkMapping {
				entry := map[string]interface{}{
					"sourceNetwork":      m.SourceNetwork,
					"destinationNetwork": m.DestinationNetwork,
				}
				if m.NetworkInterfaceModel != "" {
					entry["networkInterfaceModel"] = m.NetworkInterfaceModel
				}
				mappings = append(mappings, entry)
			}
			if len(mappings) == 0 {
				unstructured.RemoveNestedField(item.Object, "spec", "networkMapping")
			} else {
				if err := unstructured.SetNestedSlice(item.Object, mappings, "spec", "networkMapping"); err != nil {
					httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to set network mapping: "+err.Error())
					return
				}
			}
		}

		updatedItem, err := clients.Dynamic.Resource(kube.VMIGVR).Namespace(namespace).Update(context.TODO(), item, metav1.UpdateOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to update plan: "+err.Error())
			return
		}

		// The vm-import-controller treats virtualMachineImportInvalid (and other
		// terminal states) as final: its reconcile switch returns early and never
		// re-runs preflight on a spec change. Only an empty status hits the `case ""`
		// branch that re-validates. So after saving the edited spec we clear
		// status.importStatus via the status subresource to force re-reconciliation;
		// without this an edit silently leaves the plan stuck in its old state.
		kube.SetNested(updatedItem.Object, "", "status", "importStatus")
		finalItem, err := clients.Dynamic.Resource(kube.VMIGVR).Namespace(namespace).UpdateStatus(context.TODO(), updatedItem, metav1.UpdateOptions{})
		if err != nil {
			// Spec saved but status reset failed — the plan may stay in its terminal
			// state until recreated. Surface a warning rather than failing the edit.
			log.Warnf("Plan %s/%s spec updated but status reset failed (plan may remain invalid until recreated): %v", namespace, name, err)
			httpx.RespondWithJSON(w, http.StatusOK, updatedItem)
			return
		}

		httpx.RespondWithJSON(w, http.StatusOK, finalItem)
	}
}

func RunPlan(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		log.Infof("Triggering 'Run Now' for VirtualMachineImport CR: %s in namespace %s", name, namespace)

		item, err := clients.Dynamic.Resource(kube.VMIGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		unstructured.RemoveNestedField(item.Object, "spec", "schedule")

		updatedItem, err := clients.Dynamic.Resource(kube.VMIGVR).Namespace(namespace).Update(context.TODO(), item, metav1.UpdateOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusOK, updatedItem)
	}
}

func GetPlanLogs(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		log.Infof("Fetching logs related to plan %s/%s", namespace, name)

		// 1. Get the plan to find its source
		planObj, err := clients.Dynamic.Resource(kube.VMIGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to get plan: "+err.Error())
			return
		}
		sourceName, _ := kube.NestedStringOrWarn(planObj.Object, "spec", "sourceCluster", "name")
		sourceNamespace, _ := kube.NestedStringOrWarn(planObj.Object, "spec", "sourceCluster", "namespace")

		// 2. Find the controller pod
		pods, err := clients.Clientset.CoreV1().Pods("harvester-system").List(context.TODO(), metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=harvester-vm-import-controller",
		})
		if err != nil || len(pods.Items) == 0 {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Could not find vm-import-controller pod")
			return
		}
		podName := pods.Items[0].Name

		// 3. Fetch logs from the pod
		req := clients.Clientset.CoreV1().Pods("harvester-system").GetLogs(podName, &v1.PodLogOptions{})
		podLogs, err := req.Stream(context.TODO())
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to stream pod logs: "+err.Error())
			return
		}
		defer podLogs.Close()

		// 4. Filter logs for the specific plan and its source
		var logOutput strings.Builder
		scanner := bufio.NewScanner(podLogs)
		planSearchString := fmt.Sprintf("'%s/%s'", namespace, name)
		sourceSearchString := fmt.Sprintf("'%s/%s'", sourceNamespace, sourceName)
		showAll := r.URL.Query().Get("all") == "true"

		for scanner.Scan() {
			line := scanner.Text()
			if showAll || strings.Contains(line, planSearchString) || strings.Contains(line, sourceSearchString) {
				logOutput.WriteString(line + "\n")
			}
		}

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(logOutput.String())); err != nil {
			log.Warnf("Failed to write response: %v", err)
		}
	}
}

func GetPlanYAML(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		log.Infof("Fetching YAML for plan %s/%s", namespace, name)

		item, err := clients.Dynamic.Resource(kube.VMIGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithAPIError(w, err)
			return
		}

		// Convert unstructured object to YAML
		yamlBytes, err := yaml.Marshal(item.Object)
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to marshal plan to YAML: "+err.Error())
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
