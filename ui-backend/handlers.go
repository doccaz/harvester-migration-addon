// pkg/handlers.go
package main

import (
	"encoding/json"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	vmiGVR = schema.GroupVersionResource{
		Group:    "migration.harvesterhci.io",
		Version:  "v1beta1",
		Resource: "virtualmachineimports",
	}
	vmwareSourceGVR = schema.GroupVersionResource{
		Group:    "migration.harvesterhci.io",
		Version:  "v1beta1",
		Resource: "vmwaresources",
	}
	ovaSourceGVR = schema.GroupVersionResource{
		Group:    "migration.harvesterhci.io",
		Version:  "v1beta1",
		Resource: "ovasources",
	}
	vmGVR = schema.GroupVersionResource{
		Group:    "kubevirt.io",
		Version:  "v1",
		Resource: "virtualmachines",
	}
	// NEW: To check cluster version
	settingsGVR = schema.GroupVersionResource{
		Group:    "harvesterhci.io",
		Version:  "v1beta1",
		Resource: "settings",
	}

	// Forklift GVRs
	forkliftProviderGVR = schema.GroupVersionResource{
		Group:    "forklift.konveyor.io",
		Version:  "v1beta1",
		Resource: "providers",
	}
	forkliftPlanGVR = schema.GroupVersionResource{
		Group:    "forklift.konveyor.io",
		Version:  "v1beta1",
		Resource: "plans",
	}
	forkliftNetworkMapGVR = schema.GroupVersionResource{
		Group:    "forklift.konveyor.io",
		Version:  "v1beta1",
		Resource: "networkmaps",
	}
	forkliftStorageMapGVR = schema.GroupVersionResource{
		Group:    "forklift.konveyor.io",
		Version:  "v1beta1",
		Resource: "storagemaps",
	}
	forkliftMigrationGVR = schema.GroupVersionResource{
		Group:    "forklift.konveyor.io",
		Version:  "v1beta1",
		Resource: "migrations",
	}
)

// Helper to respond with JSON
func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("Failed to marshal JSON response: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if _, writeErr := w.Write([]byte(`{"error":"internal server error: failed to marshal response"}`)); writeErr != nil {
			log.Warnf("Failed to write error response: %v", writeErr)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if _, writeErr := w.Write(response); writeErr != nil {
		log.Warnf("Failed to write response body: %v", writeErr)
	}
}

// Helper to respond with a JSON error
func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}

// getNestedStringOrWarn extracts a nested string from an unstructured object.
// Returns the value and true if found, or "" and false (with a debug log) if missing.
func getNestedStringOrWarn(obj map[string]interface{}, fields ...string) (string, bool) {
	val, found, err := unstructured.NestedString(obj, fields...)
	if err != nil {
		log.Warnf("Error reading field %v: %v", fields, err)
		return "", false
	}
	if !found {
		log.Debugf("Field %v not found in object", fields)
		return "", false
	}
	return val, true
}

// setNested sets a field on an unstructured object. It can only fail when an
// intermediate path element is not a map, which the CRDs never produce; the
// failure is logged instead of silently dropped.
func setNested(obj map[string]interface{}, val interface{}, path ...string) {
	if err := unstructured.SetNestedField(obj, val, path...); err != nil {
		log.Warnf("could not set %s: %v", strings.Join(path, "."), err)
	}
}
