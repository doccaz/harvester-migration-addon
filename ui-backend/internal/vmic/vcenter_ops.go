// vcenter_ops.go
package vmic

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/vcenter"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/httpx"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetInventory(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		log.Infof("Fetching inventory for VmwareSource %s/%s", namespace, name)

		inventory, err := vcenter.GatherInventory(r.Context(), clients, namespace, name)
		if err != nil {
			log.Errorf("Failed to get vCenter inventory: %v", err)
			httpx.RespondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusOK, inventory)
	}
}

type VirtualMachinePowerRequest struct {
	VMName    string `json:"vmName"`
	Operation string `json:"operation"` // "on", "off", "reset", "shutdown"
}

func PowerOp(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		var req VirtualMachinePowerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		log.Infof("Power operation '%s' requested for VM %s via VmwareSource %s/%s", req.Operation, req.VMName, namespace, name)

		sourceObj, err := clients.Dynamic.Resource(kube.VMwareSourceGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to get VmwareSource: "+err.Error())
			return
		}

		endpoint, found := kube.NestedStringOrWarn(sourceObj.Object, "spec", "endpoint")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing spec.endpoint")
			return
		}
		datacenter, _ := kube.NestedStringOrWarn(sourceObj.Object, "spec", "dc")
		secretName, found := kube.NestedStringOrWarn(sourceObj.Object, "spec", "credentials", "name")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing credentials secret name")
			return
		}
		secretNamespace, found := kube.NestedStringOrWarn(sourceObj.Object, "spec", "credentials", "namespace")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing credentials secret namespace")
			return
		}

		secret, err := clients.Clientset.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to get credentials secret: "+err.Error())
			return
		}

		creds := vcenter.Credentials{
			URL:        endpoint,
			Username:   string(secret.Data["username"]),
			Password:   string(secret.Data["password"]),
			Datacenter: datacenter,
		}

		if err := vcenter.PowerOpVM(r.Context(), creds, req.VMName, req.Operation); err != nil {
			log.Errorf("Failed to perform power operation: %v", err)
			httpx.RespondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Power operation successful"})
	}
}

type VirtualMachineRenameRequest struct {
	OldName string `json:"oldName"`
	NewName string `json:"newName"`
}

func RenameVM(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		var req VirtualMachineRenameRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		log.Infof("Rename operation requested from '%s' to '%s' via VmwareSource %s/%s", req.OldName, req.NewName, namespace, name)

		sourceObj, err := clients.Dynamic.Resource(kube.VMwareSourceGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to get VmwareSource: "+err.Error())
			return
		}

		endpoint, found := kube.NestedStringOrWarn(sourceObj.Object, "spec", "endpoint")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing spec.endpoint")
			return
		}
		datacenter, _ := kube.NestedStringOrWarn(sourceObj.Object, "spec", "dc")
		secretName, found := kube.NestedStringOrWarn(sourceObj.Object, "spec", "credentials", "name")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing credentials secret name")
			return
		}
		secretNamespace, found := kube.NestedStringOrWarn(sourceObj.Object, "spec", "credentials", "namespace")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing credentials secret namespace")
			return
		}

		secret, err := clients.Clientset.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to get credentials secret: "+err.Error())
			return
		}

		creds := vcenter.Credentials{
			URL:        endpoint,
			Username:   string(secret.Data["username"]),
			Password:   string(secret.Data["password"]),
			Datacenter: datacenter,
		}

		if err := vcenter.RenameVM(r.Context(), creds, req.OldName, req.NewName); err != nil {
			log.Errorf("Failed to rename VM: %v", err)
			httpx.RespondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Rename successful"})
	}
}

type UpdateVMMACRequest struct {
	VMName    string `json:"vmName"`
	DeviceKey int32  `json:"deviceKey"`
	NewMAC    string `json:"newMac"`
}

func UpdateMAC(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		var req UpdateVMMACRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		log.Infof("MAC address update requested for VM '%s' (device %d) to '%s' via VmwareSource %s/%s", req.VMName, req.DeviceKey, req.NewMAC, namespace, name)

		sourceObj, err := clients.Dynamic.Resource(kube.VMwareSourceGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to get VmwareSource: "+err.Error())
			return
		}

		endpoint, found := kube.NestedStringOrWarn(sourceObj.Object, "spec", "endpoint")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing spec.endpoint")
			return
		}
		datacenter, _ := kube.NestedStringOrWarn(sourceObj.Object, "spec", "dc")
		secretName, found := kube.NestedStringOrWarn(sourceObj.Object, "spec", "credentials", "name")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing credentials secret name")
			return
		}
		secretNamespace, found := kube.NestedStringOrWarn(sourceObj.Object, "spec", "credentials", "namespace")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing credentials secret namespace")
			return
		}

		secret, err := clients.Clientset.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to get credentials secret: "+err.Error())
			return
		}

		creds := vcenter.Credentials{
			URL:        endpoint,
			Username:   string(secret.Data["username"]),
			Password:   string(secret.Data["password"]),
			Datacenter: datacenter,
		}

		if err := vcenter.UpdateVMNetworkMAC(r.Context(), creds, req.VMName, req.DeviceKey, req.NewMAC); err != nil {
			log.Errorf("Failed to update VM MAC: %v", err)
			httpx.RespondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "MAC address updated successfully"})
	}
}

// --- Forklift Handlers ---
