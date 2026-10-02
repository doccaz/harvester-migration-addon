// unstructured_test.go
package kube

import (
	"testing"
)

func TestNestedStringOrWarn(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"type": "vsphere",
			"url":  "https://vcenter.example.com",
		},
	}

	t.Run("found", func(t *testing.T) {
		val, ok := NestedStringOrWarn(obj, "spec", "type")
		if !ok || val != "vsphere" {
			t.Errorf("expected ('vsphere', true), got ('%s', %v)", val, ok)
		}
	})

	t.Run("missing", func(t *testing.T) {
		val, ok := NestedStringOrWarn(obj, "spec", "nonexistent")
		if ok || val != "" {
			t.Errorf("expected ('', false), got ('%s', %v)", val, ok)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		// Nested field exists but is not a string
		obj["spec"].(map[string]interface{})["count"] = 42
		val, ok := NestedStringOrWarn(obj, "spec", "count")
		if ok || val != "" {
			t.Errorf("expected ('', false) for non-string field, got ('%s', %v)", val, ok)
		}
	})
}
