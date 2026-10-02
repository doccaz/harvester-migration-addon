// forklift_providers.go
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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
			httpx.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
				"available":        false,
				"defaultNamespace": namespace,
				"message":          "Forklift host Provider not found in namespace " + namespace + ". Forklift features are unavailable.",
			})
			return
		}

		httpx.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
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
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to list Forklift Providers: "+err.Error())
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
		httpx.RespondWithJSON(w, http.StatusOK, sourceProviders)
	}
}

// CreateForkliftProviderHandler creates a Forklift Provider with its associated Secret
func CreateForkliftProviderHandler(clients *K8sClients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload CreateForkliftProviderPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			httpx.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
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
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to create Forklift secret: "+err.Error())
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
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to create Forklift Provider: "+err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusCreated, createdObj)
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
			httpx.RespondWithError(w, http.StatusNotFound, "Failed to get Forklift Provider: "+err.Error())
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

		httpx.RespondWithJSON(w, http.StatusOK, providerObj)
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
			httpx.RespondWithError(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		providerObj, err := clients.Dynamic.Resource(forkliftProviderGVR).Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusNotFound, "Failed to get Forklift Provider: "+err.Error())
			return
		}

		// Update secret if credentials or TLS settings provided
		secretName, found := getNestedStringOrWarn(providerObj.Object, "spec", "secret", "name")
		if !found {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Forklift Provider missing secret name")
			return
		}
		needsSecretUpdate := payload.Username != "" || payload.Password != "" ||
			payload.URL != "" || payload.InsecureSkipVerify != nil || payload.CACert != ""
		if needsSecretUpdate {
			secret, err := clients.Clientset.CoreV1().Secrets(namespace).Get(context.TODO(), secretName, metav1.GetOptions{})
			if err != nil {
				httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to get associated secret: "+err.Error())
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
				httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to update secret: "+err.Error())
				return
			}
		}

		// Update the Provider URL
		if payload.URL != "" {
			if err := unstructured.SetNestedField(providerObj.Object, payload.URL, "spec", "url"); err != nil {
				httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to set URL: "+err.Error())
				return
			}
		}

		// Update sdkEndpoint setting
		if payload.SdkEndpoint != "" {
			if err := unstructured.SetNestedField(providerObj.Object, payload.SdkEndpoint, "spec", "settings", "sdkEndpoint"); err != nil {
				httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to set sdkEndpoint: "+err.Error())
				return
			}
		}

		// Update VDDK init image
		if payload.VddkInitImage != "" {
			if err := unstructured.SetNestedField(providerObj.Object, payload.VddkInitImage, "spec", "settings", "vddkInitImage"); err != nil {
				httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to set vddkInitImage: "+err.Error())
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
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to update Forklift Provider: "+err.Error())
			return
		}

		httpx.RespondWithJSON(w, http.StatusOK, updatedObj)
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
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to get Forklift Provider: "+err.Error())
			return
		}
		secretName, _ := getNestedStringOrWarn(providerObj.Object, "spec", "secret", "name")

		err = clients.Dynamic.Resource(forkliftProviderGVR).Namespace(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
		if err != nil {
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to delete Forklift Provider: "+err.Error())
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
