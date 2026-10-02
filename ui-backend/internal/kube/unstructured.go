// unstructured.go
package kube

import (
	"strings"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// NestedStringOrWarn extracts a nested string from an unstructured object.
// Returns the value and true if found, or "" and false (with a debug log) if missing.
func NestedStringOrWarn(obj map[string]interface{}, fields ...string) (string, bool) {
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

// SetNested sets a field on an unstructured object. It can only fail when an
// intermediate path element is not a map, which the CRDs never produce; the
// failure is logged instead of silently dropped.
func SetNested(obj map[string]interface{}, val interface{}, path ...string) {
	if err := unstructured.SetNestedField(obj, val, path...); err != nil {
		log.Warnf("could not set %s: %v", strings.Join(path, "."), err)
	}
}
