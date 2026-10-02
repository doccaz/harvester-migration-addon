// plans_test.go
package vmic

import (
	"context"
	"net/http"
	"testing"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"
	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestUpdatePlan(t *testing.T) {
	scheme := runtime.NewScheme()
	gvrToListKind := map[schema.GroupVersionResource]string{
		kube.VMIGVR: "VirtualMachineImportList",
	}

	plan := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "migration.harvesterhci.io/v1beta1",
		"kind":       "VirtualMachineImport",
		"metadata": map[string]interface{}{
			"name":      "stuck-plan",
			"namespace": "techday",
		},
		"spec": map[string]interface{}{
			"virtualMachineName": "OLD NAME",
			"storageClass":       "old-sc",
			"folder":             "/dc/old",
		},
		"status": map[string]interface{}{
			"importStatus":               "virtualMachineImportInvalid",
			"importedVirtualMachineName": "old name",
		},
	}}

	fakeDynamic := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, plan)
	clients := &kube.Clients{Clientset: fake.NewSimpleClientset(), Dynamic: fakeDynamic}

	newName := "sles16"
	newSC := "harvester-1replica"
	emptyFolder := ""
	payload := UpdatePlanPayload{
		VirtualMachineName: &newName,
		StorageClass:       &newSC,
		Folder:             &emptyFolder, // empty clears the field
	}

	rr := testutil.Do(UpdatePlan(clients), http.MethodPut,
		"/api/v1/plans/techday/stuck-plan", payload,
		map[string]string{"namespace": "techday", "name": "stuck-plan"})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	got, err := fakeDynamic.Resource(kube.VMIGVR).Namespace("techday").Get(context.TODO(), "stuck-plan", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get updated plan: %v", err)
	}

	if name, _, _ := unstructured.NestedString(got.Object, "spec", "virtualMachineName"); name != "sles16" {
		t.Errorf("expected virtualMachineName 'sles16', got '%s'", name)
	}
	if sc, _, _ := unstructured.NestedString(got.Object, "spec", "storageClass"); sc != "harvester-1replica" {
		t.Errorf("expected storageClass 'harvester-1replica', got '%s'", sc)
	}
	if _, found, _ := unstructured.NestedString(got.Object, "spec", "folder"); found {
		t.Errorf("expected folder to be cleared, but it is still present")
	}
	// The crux: status.importStatus must be reset so the controller re-runs preflight.
	if status, _, _ := unstructured.NestedString(got.Object, "status", "importStatus"); status != "" {
		t.Errorf("expected importStatus reset to empty, got '%s'", status)
	}
}

func TestGetPlanYAML(t *testing.T) {
	plan := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "migration.harvesterhci.io/v1beta1", "kind": "VirtualMachineImport",
		"metadata": map[string]interface{}{"name": "p1", "namespace": "ns"},
	}}
	clients := testutil.NewClientsWithDynamic(nil, []runtime.Object{plan}...)

	rr := testutil.Do(GetPlanYAML(clients), "GET", "/x", nil, map[string]string{"namespace": "ns", "name": "p1"})
	if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "application/yaml" {
		t.Fatalf("existing plan: %d %q", rr.Code, rr.Header().Get("Content-Type"))
	}
	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("YAML must be served with X-Content-Type-Options: nosniff")
	}
	rr = testutil.Do(GetPlanYAML(clients), "GET", "/x", nil, map[string]string{"namespace": "ns", "name": "missing"})
	if rr.Code != http.StatusNotFound {
		t.Errorf("missing plan: status %d, want 404", rr.Code)
	}
}
