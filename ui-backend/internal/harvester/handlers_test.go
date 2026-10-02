// handlers_test.go
package harvester

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"
	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

func namespace(name string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
}

func obj(apiVersion, kind, ns, name string, labels map[string]string) *unstructured.Unstructured {
	meta := map[string]interface{}{"name": name}
	if ns != "" {
		meta["namespace"] = ns
	}
	if labels != nil {
		l := map[string]interface{}{}
		for k, v := range labels {
			l[k] = v
		}
		meta["labels"] = l
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion, "kind": kind, "metadata": meta,
	}}
}

func names(t *testing.T, body []byte) []string {
	t.Helper()
	var items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("not a JSON array of objects: %v\n%s", err, body)
	}
	out := make([]string, 0, len(items))
	for _, i := range items {
		out = append(out, i.Metadata.Name)
	}
	return out
}

func TestListNamespaces(t *testing.T) {
	clients := testutil.NewClients(namespace("default"), namespace("forklift"))
	rr := testutil.Do(ListNamespaces(clients), "GET", "/api/v1/harvester/namespaces", nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	if got := strings.Join(names(t, rr.Body.Bytes()), ","); got != "default,forklift" {
		t.Errorf("namespaces = %q", got)
	}
}

func TestCreateNamespace(t *testing.T) {
	t.Run("creates it", func(t *testing.T) {
		clients := testutil.NewClients()
		rr := testutil.Do(CreateNamespace(clients), "POST", "/api/v1/harvester/namespaces", map[string]string{"name": "migrations"}, nil)
		if rr.Code != http.StatusCreated {
			t.Fatalf("status %d: %s", rr.Code, rr.Body)
		}
		if _, err := clients.Clientset.CoreV1().Namespaces().Get(t.Context(), "migrations", metav1.GetOptions{}); err != nil {
			t.Errorf("namespace not created: %v", err)
		}
		var body map[string]string
		_ = json.Unmarshal(rr.Body.Bytes(), &body)
		if body["status"] != "namespace created" {
			t.Errorf("body = %v", body)
		}
	})
	t.Run("rejects a malformed body", func(t *testing.T) {
		rr := testutil.Do(CreateNamespace(testutil.NewClients()), "POST", "/x", "not json{", nil)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("status %d, want 400", rr.Code)
		}
	})
	// Current behaviour, pinned: any failure from the API server, including
	// "already exists", surfaces as 500 rather than 409.
	t.Run("an existing namespace is reported as 500", func(t *testing.T) {
		clients := testutil.NewClients(namespace("taken"))
		rr := testutil.Do(CreateNamespace(clients), "POST", "/x", map[string]string{"name": "taken"}, nil)
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("status %d", rr.Code)
		}
	})
}

func TestListVlanConfigsOnlyReturnsHarvesterManagedNetworks(t *testing.T) {
	const typ = "network.harvesterhci.io/type"
	nadGVR := schema.GroupVersionResource{Group: "k8s.cni.cncf.io", Version: "v1", Resource: "network-attachment-definitions"}
	clients := testutil.NewClientsWithListKinds(map[schema.GroupVersionResource]string{nadGVR: "NetworkAttachmentDefinitionList"})
	for _, n := range []*unstructured.Unstructured{
		obj("k8s.cni.cncf.io/v1", "NetworkAttachmentDefinition", "default", "vlan100", map[string]string{typ: "L2VlanNetwork"}),
		// UntaggedNetwork NADs (e.g. "local-network") must be included too.
		obj("k8s.cni.cncf.io/v1", "NetworkAttachmentDefinition", "default", "local-network", map[string]string{typ: "UntaggedNetwork"}),
		obj("k8s.cni.cncf.io/v1", "NetworkAttachmentDefinition", "default", "someone-elses", nil),
	} {
		if _, err := clients.Dynamic.Resource(nadGVR).Namespace("default").Create(t.Context(), n, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	rr := testutil.Do(ListVlanConfigs(clients), "GET", "/api/v1/harvester/vlanconfigs", nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body)
	}
	got := names(t, rr.Body.Bytes())
	sort.Strings(got)
	if strings.Join(got, ",") != "local-network,vlan100" {
		t.Errorf("NADs = %v, want only the two Harvester-labelled ones", got)
	}
}

func TestListStorageClasses(t *testing.T) {
	clients := testutil.NewClients(
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "harvester-longhorn"}, Provisioner: "driver.longhorn.io"},
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "nfs"}, Provisioner: "nfs.csi.k8s.io"},
	)
	rr := testutil.Do(ListStorageClasses(clients), "GET", "/api/v1/harvester/storageclasses", nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	if got := strings.Join(names(t, rr.Body.Bytes()), ","); got != "harvester-longhorn,nfs" {
		t.Errorf("storage classes = %q", got)
	}
}

func TestListVMsIsScopedToTheNamespace(t *testing.T) {
	kv := "kubevirt.io/v1"
	clients := testutil.NewClientsWithDynamic(nil,
		obj(kv, "VirtualMachine", "prod", "web", nil),
		obj(kv, "VirtualMachine", "prod", "db", nil),
		obj(kv, "VirtualMachine", "lab", "scratch", nil),
	)
	rr := testutil.Do(ListVMs(clients), "GET", "/api/v1/harvester/virtualmachines/prod", nil, map[string]string{"namespace": "prod"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body)
	}
	got := names(t, rr.Body.Bytes())
	if len(got) != 2 || strings.Contains(strings.Join(got, ","), "scratch") {
		t.Errorf("VMs in prod = %v", got)
	}
}

func TestListVMsInAnEmptyNamespace(t *testing.T) {
	clients := testutil.NewClientsWithListKinds(map[schema.GroupVersionResource]string{kube.VMGVR: "VirtualMachineList"})
	rr := testutil.Do(ListVMs(clients), "GET", "/x", nil, map[string]string{"namespace": "none"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	// The fake client serialises an empty list as null where a real API server
	// returns []; both decode to an empty list, which is all the UI relies on.
	var items []json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &items); err != nil || len(items) != 0 {
		t.Errorf("body %q: err=%v items=%d", rr.Body.String(), err, len(items))
	}
}

func vmwareSource() *unstructured.Unstructured {
	u := obj("migration.harvesterhci.io/v1beta1", "VmwareSource", "default", "vc1", nil)
	u.Object["spec"] = map[string]interface{}{"endpoint": "https://vc.example.com/sdk", "dc": "DC1"}
	return u
}

func TestGetResource(t *testing.T) {
	clients := testutil.NewClientsWithDynamic(nil, []runtime.Object{vmwareSource()}...)
	vars := map[string]string{"namespace": "default", "name": "vc1"}

	rr := testutil.Do(GetResource(clients, kube.VMwareSourceGVR), "GET", "/x", nil, vars)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body)
	}
	var got map[string]interface{}
	_ = json.Unmarshal(rr.Body.Bytes(), &got)
	if got["spec"].(map[string]interface{})["dc"] != "DC1" {
		t.Errorf("body = %v", got)
	}

	// Any lookup failure is reported as 404.
	rr = testutil.Do(GetResource(clients, kube.VMwareSourceGVR), "GET", "/x", nil, map[string]string{"namespace": "default", "name": "nope"})
	if rr.Code != http.StatusNotFound {
		t.Errorf("missing resource: status %d, want 404", rr.Code)
	}
}

func TestGetSourceYAML(t *testing.T) {
	clients := testutil.NewClientsWithDynamic(nil, []runtime.Object{vmwareSource()}...)

	rr := testutil.Do(GetSourceYAML(clients, kube.VMwareSourceGVR), "GET", "/x", nil, map[string]string{"namespace": "default", "name": "vc1"})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("Content-Type = %q", ct)
	}
	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("YAML must be served with X-Content-Type-Options: nosniff")
	}
	var back map[string]interface{}
	if err := yaml.Unmarshal(rr.Body.Bytes(), &back); err != nil {
		t.Fatalf("response is not YAML: %v", err)
	}
	if back["kind"] != "VmwareSource" {
		t.Errorf("kind = %v", back["kind"])
	}

	// Known inconsistency, pinned until it is changed deliberately: a missing
	// object is a 500 here, while GetResource answers 404 (see docs/refactor-notes.md).
	rr = testutil.Do(GetSourceYAML(clients, kube.VMwareSourceGVR), "GET", "/x", nil, map[string]string{"namespace": "default", "name": "nope"})
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("missing object: status %d (currently 500)", rr.Code)
	}
}
