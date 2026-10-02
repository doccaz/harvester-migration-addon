// vcenter_handlers.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/httpx"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type VCenterCredentials struct {
	URL        string
	Username   string
	Password   string
	Datacenter string
}

// gatherVCenterInventory resolves a VmwareSource's endpoint and credentials and
// returns its inventory tree. Shared by the inventory endpoint and the support
// bundle so both go through one code path.
func gatherVCenterInventory(ctx context.Context, clients *K8sClients, namespace, name string) (*InventoryNode, error) {
	sourceObj, err := clients.Dynamic.Resource(vmwareSourceGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get VmwareSource: %w", err)
	}

	endpoint, found := getNestedStringOrWarn(sourceObj.Object, "spec", "endpoint")
	if !found {
		return nil, fmt.Errorf("VmwareSource missing spec.endpoint")
	}
	datacenter, _ := getNestedStringOrWarn(sourceObj.Object, "spec", "dc")

	secretName, found := getNestedStringOrWarn(sourceObj.Object, "spec", "credentials", "name")
	if !found {
		return nil, fmt.Errorf("VmwareSource missing credentials secret name")
	}
	secretNamespace, found := getNestedStringOrWarn(sourceObj.Object, "spec", "credentials", "namespace")
	if !found {
		return nil, fmt.Errorf("VmwareSource missing credentials secret namespace")
	}

	secret, err := clients.Clientset.CoreV1().Secrets(secretNamespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials secret: %w", err)
	}

	creds := VCenterCredentials{
		URL:        endpoint,
		Username:   string(secret.Data["username"]),
		Password:   string(secret.Data["password"]),
		Datacenter: datacenter,
	}

	return GetVCenterInventory(ctx, creds)
}

func HandleGetInventory(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		log.Infof("Fetching inventory for VmwareSource %s/%s", namespace, name)

		inventory, err := gatherVCenterInventory(r.Context(), clients, namespace, name)
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

func HandleVMPowerOp(clients *K8sClients) http.HandlerFunc {
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

		sourceObj, err := clients.Dynamic.Resource(vmwareSourceGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to get VmwareSource: "+err.Error())
			return
		}

		endpoint, found := getNestedStringOrWarn(sourceObj.Object, "spec", "endpoint")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing spec.endpoint")
			return
		}
		datacenter, _ := getNestedStringOrWarn(sourceObj.Object, "spec", "dc")
		secretName, found := getNestedStringOrWarn(sourceObj.Object, "spec", "credentials", "name")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing credentials secret name")
			return
		}
		secretNamespace, found := getNestedStringOrWarn(sourceObj.Object, "spec", "credentials", "namespace")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing credentials secret namespace")
			return
		}

		secret, err := clients.Clientset.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to get credentials secret: "+err.Error())
			return
		}

		creds := VCenterCredentials{
			URL:        endpoint,
			Username:   string(secret.Data["username"]),
			Password:   string(secret.Data["password"]),
			Datacenter: datacenter,
		}

		if err := PowerOpVM(r.Context(), creds, req.VMName, req.Operation); err != nil {
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

func HandleVMRename(clients *K8sClients) http.HandlerFunc {
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

		sourceObj, err := clients.Dynamic.Resource(vmwareSourceGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to get VmwareSource: "+err.Error())
			return
		}

		endpoint, found := getNestedStringOrWarn(sourceObj.Object, "spec", "endpoint")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing spec.endpoint")
			return
		}
		datacenter, _ := getNestedStringOrWarn(sourceObj.Object, "spec", "dc")
		secretName, found := getNestedStringOrWarn(sourceObj.Object, "spec", "credentials", "name")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing credentials secret name")
			return
		}
		secretNamespace, found := getNestedStringOrWarn(sourceObj.Object, "spec", "credentials", "namespace")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing credentials secret namespace")
			return
		}

		secret, err := clients.Clientset.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to get credentials secret: "+err.Error())
			return
		}

		creds := VCenterCredentials{
			URL:        endpoint,
			Username:   string(secret.Data["username"]),
			Password:   string(secret.Data["password"]),
			Datacenter: datacenter,
		}

		if err := RenameVM(r.Context(), creds, req.OldName, req.NewName); err != nil {
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

func HandleUpdateVMMAC(clients *K8sClients) http.HandlerFunc {
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

		sourceObj, err := clients.Dynamic.Resource(vmwareSourceGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to get VmwareSource: "+err.Error())
			return
		}

		endpoint, found := getNestedStringOrWarn(sourceObj.Object, "spec", "endpoint")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing spec.endpoint")
			return
		}
		datacenter, _ := getNestedStringOrWarn(sourceObj.Object, "spec", "dc")
		secretName, found := getNestedStringOrWarn(sourceObj.Object, "spec", "credentials", "name")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing credentials secret name")
			return
		}
		secretNamespace, found := getNestedStringOrWarn(sourceObj.Object, "spec", "credentials", "namespace")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing credentials secret namespace")
			return
		}

		secret, err := clients.Clientset.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to get credentials secret: "+err.Error())
			return
		}

		creds := VCenterCredentials{
			URL:        endpoint,
			Username:   string(secret.Data["username"]),
			Password:   string(secret.Data["password"]),
			Datacenter: datacenter,
		}

		if err := UpdateVMNetworkMAC(r.Context(), creds, req.VMName, req.DeviceKey, req.NewMAC); err != nil {
			log.Errorf("Failed to update VM MAC: %v", err)
			httpx.RespondWithError(w, http.StatusInternalServerError, err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "MAC address updated successfully"})
	}
}

// --- Forklift Handlers ---
