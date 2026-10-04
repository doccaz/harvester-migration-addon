// sources.go
package vmic

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/httpx"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func ListVmwareSources(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := clients.Dynamic.Resource(kube.VMwareSourceGVR).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			httpx.RespondWithAPIErrorMsg(w, err, "Failed to list VmwareSource CRs: "+err.Error())
			return
		}
		httpx.RespondWithJSON(w, http.StatusOK, list.Items)
	}
}

type CreateVmwareSourcePayload struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Endpoint   string `json:"endpoint"`
	Datacenter string `json:"datacenter"`
	Username   string `json:"username"`
	Password   string `json:"password"`
}

func CreateVmwareSource(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload CreateVmwareSourcePayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			httpx.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		// 1. Create the Secret
		secretName := payload.Name + "-credentials"
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: payload.Namespace,
			},
			StringData: map[string]string{
				"username": payload.Username,
				"password": payload.Password,
			},
		}
		_, err := clients.Clientset.CoreV1().Secrets(payload.Namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
		if err != nil {
			httpx.RespondWithAPIErrorMsg(w, err, "Failed to create credentials secret: "+err.Error())
			return
		}

		// 2. Create the VmwareSource
		vmwareSource := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "migration.harvesterhci.io/v1beta1",
				"kind":       "VmwareSource",
				"metadata": map[string]interface{}{
					"name":      payload.Name,
					"namespace": payload.Namespace,
				},
				"spec": map[string]interface{}{
					"endpoint": payload.Endpoint,
					"dc":       payload.Datacenter,
					"credentials": map[string]interface{}{
						"name":      secretName,
						"namespace": payload.Namespace,
					},
				},
			},
		}

		createdObj, err := clients.Dynamic.Resource(kube.VMwareSourceGVR).Namespace(payload.Namespace).Create(context.TODO(), vmwareSource, metav1.CreateOptions{})
		if err != nil {
			// Clean up the secret if source creation fails
			if cleanupErr := clients.Clientset.CoreV1().Secrets(payload.Namespace).Delete(context.TODO(), secretName, metav1.DeleteOptions{}); cleanupErr != nil {
				log.Warnf("Best-effort cleanup: failed to delete secret %s/%s: %v", payload.Namespace, secretName, cleanupErr)
			}
			httpx.RespondWithAPIErrorMsg(w, err, "Failed to create VmwareSource CR: "+err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusCreated, createdObj)
	}
}

func GetVmwareSource(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		sourceObj, err := clients.Dynamic.Resource(kube.VMwareSourceGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusNotFound, "Failed to get VmwareSource: "+err.Error())
			return
		}

		secretName, found := kube.NestedStringOrWarn(sourceObj.Object, "spec", "credentials", "name")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing credentials secret name")
			return
		}
		secret, err := clients.Clientset.CoreV1().Secrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithAPIErrorMsg(w, err, "Failed to get associated secret: "+err.Error())
			return
		}

		sourceObj.Object["spec"].(map[string]interface{})["username"] = string(secret.Data["username"])

		httpx.RespondWithJSON(w, http.StatusOK, sourceObj)
	}
}

func UpdateVmwareSource(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		var payload CreateVmwareSourcePayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			httpx.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		// 1. Get the existing VmwareSource to find the secret name
		sourceObj, err := clients.Dynamic.Resource(kube.VMwareSourceGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusNotFound, "Failed to get VmwareSource: "+err.Error())
			return
		}
		secretName, found := kube.NestedStringOrWarn(sourceObj.Object, "spec", "credentials", "name")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "VmwareSource missing credentials secret name")
			return
		}

		// 2. Update the Secret, only if new credentials are provided
		if payload.Username != "" || payload.Password != "" {
			secret, err := clients.Clientset.CoreV1().Secrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
			if err != nil {
				httpx.RespondWithAPIErrorMsg(w, err, "Failed to get associated secret: "+err.Error())
				return
			}

			if secret.StringData == nil {
				secret.StringData = make(map[string]string)
			}

			if payload.Username != "" {
				secret.StringData["username"] = payload.Username
			}
			if payload.Password != "" {
				secret.StringData["password"] = payload.Password
			}
			_, err = clients.Clientset.CoreV1().Secrets(namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
			if err != nil {
				httpx.RespondWithAPIErrorMsg(w, err, "Failed to update secret: "+err.Error())
				return
			}
		}

		// 3. Update the VmwareSource
		if err := unstructured.SetNestedField(sourceObj.Object, payload.Endpoint, "spec", "endpoint"); err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to set endpoint: "+err.Error())
			return
		}
		if err := unstructured.SetNestedField(sourceObj.Object, payload.Datacenter, "spec", "dc"); err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to set datacenter: "+err.Error())
			return
		}

		updatedObj, err := clients.Dynamic.Resource(kube.VMwareSourceGVR).Namespace(namespace).Update(context.TODO(), sourceObj, metav1.UpdateOptions{})
		if err != nil {
			httpx.RespondWithAPIErrorMsg(w, err, "Failed to update VmwareSource: "+err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusOK, updatedObj)
	}
}

func DeleteVmwareSource(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		// 1. Get the VmwareSource to find the associated secret
		sourceObj, err := clients.Dynamic.Resource(kube.VMwareSourceGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithAPIErrorMsg(w, err, "Failed to get VmwareSource: "+err.Error())
			return
		}
		secretName, _ := kube.NestedStringOrWarn(sourceObj.Object, "spec", "credentials", "name")

		// 2. Delete the VmwareSource
		err = clients.Dynamic.Resource(kube.VMwareSourceGVR).Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
		if err != nil {
			httpx.RespondWithAPIErrorMsg(w, err, "Failed to delete VmwareSource: "+err.Error())
			return
		}

		// 3. Delete the associated Secret
		if secretName != "" {
			err = clients.Clientset.CoreV1().Secrets(namespace).Delete(context.TODO(), secretName, metav1.DeleteOptions{})
			if err != nil {
				// Log the error but don't fail the request, as the primary resource was deleted.
				log.Warnf("Failed to delete associated secret %s/%s: %v", namespace, secretName, err)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// --- OvaSource Handlers ---
func ListOvaSources(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := clients.Dynamic.Resource(kube.OVASourceGVR).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			httpx.RespondWithAPIErrorMsg(w, err, "Failed to list OvaSource CRs: "+err.Error())
			return
		}
		httpx.RespondWithJSON(w, http.StatusOK, list.Items)
	}
}

type CreateOvaSourcePayload struct {
	Name               string `json:"name"`
	Namespace          string `json:"namespace"`
	URL                string `json:"url"`
	HttpTimeoutSeconds int    `json:"httpTimeoutSeconds,omitempty"`
	Username           string `json:"username"`
	Password           string `json:"password"`
}

func CreateOvaSource(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload CreateOvaSourcePayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			httpx.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		// 1. Create the Secret
		secretName := payload.Name + "-ova-credentials"
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: payload.Namespace,
			},
			StringData: map[string]string{
				"username": payload.Username,
				"password": payload.Password,
			},
		}
		_, err := clients.Clientset.CoreV1().Secrets(payload.Namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
		if err != nil {
			httpx.RespondWithAPIErrorMsg(w, err, "Failed to create credentials secret: "+err.Error())
			return
		}

		// 2. Create the OvaSource
		ovaSource := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "migration.harvesterhci.io/v1beta1",
				"kind":       "OvaSource",
				"metadata": map[string]interface{}{
					"name":      payload.Name,
					"namespace": payload.Namespace,
				},
				"spec": map[string]interface{}{
					"url": payload.URL,
					"credentials": map[string]interface{}{
						"name":      secretName,
						"namespace": payload.Namespace,
					},
				},
			},
		}
		if payload.HttpTimeoutSeconds > 0 {
			ovaSource.Object["spec"].(map[string]interface{})["httpTimeoutSeconds"] = int64(payload.HttpTimeoutSeconds)
		}

		createdObj, err := clients.Dynamic.Resource(kube.OVASourceGVR).Namespace(payload.Namespace).Create(context.TODO(), ovaSource, metav1.CreateOptions{})
		if err != nil {
			if cleanupErr := clients.Clientset.CoreV1().Secrets(payload.Namespace).Delete(context.TODO(), secretName, metav1.DeleteOptions{}); cleanupErr != nil {
				log.Warnf("Best-effort cleanup: failed to delete secret %s/%s: %v", payload.Namespace, secretName, cleanupErr)
			}
			httpx.RespondWithAPIErrorMsg(w, err, "Failed to create OvaSource CR: "+err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusCreated, createdObj)
	}
}

func GetOvaSource(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		sourceObj, err := clients.Dynamic.Resource(kube.OVASourceGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusNotFound, "Failed to get OvaSource: "+err.Error())
			return
		}

		// Credentials are optional: a source made outside this UI may name no secret.
		if secretName, found, _ := unstructured.NestedString(sourceObj.Object, "spec", "credentials", "name"); found && secretName != "" {
			secret, err := clients.Clientset.CoreV1().Secrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
			if err != nil {
				httpx.RespondWithAPIErrorMsg(w, err, "Failed to get associated secret: "+err.Error())
				return
			}
			sourceObj.Object["spec"].(map[string]interface{})["username"] = string(secret.Data["username"])
		}

		httpx.RespondWithJSON(w, http.StatusOK, sourceObj)
	}
}

func UpdateOvaSource(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		var payload CreateOvaSourcePayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			httpx.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		sourceObj, err := clients.Dynamic.Resource(kube.OVASourceGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusNotFound, "Failed to get OvaSource: "+err.Error())
			return
		}
		// Credentials are optional. A source that names no secret only gets one when the
		// request supplies a username or password; then the secret is created (or a
		// leftover one of that name reused) and linked from spec.credentials.
		secretName, found, _ := unstructured.NestedString(sourceObj.Object, "spec", "credentials", "name")
		found = found && secretName != ""
		if payload.Username != "" || payload.Password != "" {
			if !found {
				secretName = name + "-ova-credentials"
				_, err := clients.Clientset.CoreV1().Secrets(namespace).Create(context.TODO(), &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
					StringData: map[string]string{"username": payload.Username, "password": payload.Password},
				}, metav1.CreateOptions{})
				switch {
				case err == nil:
				case errors.IsAlreadyExists(err):
					found = true // reuse it through the update path below
				default:
					httpx.RespondWithAPIErrorMsg(w, err, "Failed to create credentials secret: "+err.Error())
					return
				}
				if err := unstructured.SetNestedMap(sourceObj.Object, map[string]interface{}{"name": secretName, "namespace": namespace}, "spec", "credentials"); err != nil {
					httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to link the credentials: "+err.Error())
					return
				}
			}
			if found {
				secret, err := clients.Clientset.CoreV1().Secrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
				if err != nil {
					httpx.RespondWithAPIErrorMsg(w, err, "Failed to get associated secret: "+err.Error())
					return
				}

				if secret.StringData == nil {
					secret.StringData = make(map[string]string)
				}

				if payload.Username != "" {
					secret.StringData["username"] = payload.Username
				}
				if payload.Password != "" {
					secret.StringData["password"] = payload.Password
				}
				_, err = clients.Clientset.CoreV1().Secrets(namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
				if err != nil {
					httpx.RespondWithAPIErrorMsg(w, err, "Failed to update secret: "+err.Error())
					return
				}
			}
		}

		if err := unstructured.SetNestedField(sourceObj.Object, payload.URL, "spec", "url"); err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to set URL: "+err.Error())
			return
		}
		if payload.HttpTimeoutSeconds > 0 {
			if err := unstructured.SetNestedField(sourceObj.Object, int64(payload.HttpTimeoutSeconds), "spec", "httpTimeoutSeconds"); err != nil {
				log.Warnf("Failed to set httpTimeoutSeconds: %v", err)
			}
		} else {
			unstructured.RemoveNestedField(sourceObj.Object, "spec", "httpTimeoutSeconds")
		}

		updatedObj, err := clients.Dynamic.Resource(kube.OVASourceGVR).Namespace(namespace).Update(context.TODO(), sourceObj, metav1.UpdateOptions{})
		if err != nil {
			httpx.RespondWithAPIErrorMsg(w, err, "Failed to update OvaSource: "+err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusOK, updatedObj)
	}
}

func DeleteOvaSource(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		namespace := vars["namespace"]
		name := vars["name"]

		sourceObj, err := clients.Dynamic.Resource(kube.OVASourceGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithAPIErrorMsg(w, err, "Failed to get OvaSource: "+err.Error())
			return
		}
		secretName, _ := kube.NestedStringOrWarn(sourceObj.Object, "spec", "credentials", "name")

		err = clients.Dynamic.Resource(kube.OVASourceGVR).Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
		if err != nil {
			httpx.RespondWithAPIErrorMsg(w, err, "Failed to delete OvaSource: "+err.Error())
			return
		}

		if secretName != "" {
			err = clients.Clientset.CoreV1().Secrets(namespace).Delete(context.TODO(), secretName, metav1.DeleteOptions{})
			if err != nil {
				log.Warnf("Failed to delete associated secret %s/%s: %v", namespace, secretName, err)
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
