// capabilities_test.go
package capabilities

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"
	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Gather also asks each engine whether it is usable, which lists VirtualMachineImports:
// the fake needs that list kind declared.
func clientsWith(objs ...runtime.Object) *kube.Clients {
	return testutil.NewClientsWithListKindsAndObjects(map[schema.GroupVersionResource]string{kube.VMIGVR: "VirtualMachineImportList"}, objs...)
}

func serverVersion(v string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "harvesterhci.io/v1beta1",
		"kind":       "Setting",
		"metadata":   map[string]interface{}{"name": "server-version"},
		"value":      v,
	}}
}

// v1.6.0 and later unlock advanced power operations, disk bus type and preflight
// checks; the UI hides those controls on older clusters.
func TestGatherDerivesFeatureFlagsFromTheServerVersion(t *testing.T) {
	cases := []struct {
		version  string
		advanced bool
	}{
		{"v1.5.2", false},
		{"v1.4.0", false},
		{"v1.6.0", true},
		{"v1.7.1", true},
		{"v1.8.2", true},
		{"v1.9.0-rc1", true},
		{"master-head", true},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.version, func(t *testing.T) {
			clients := clientsWith(serverVersion(tc.version))
			got, err := Gather(context.Background(), clients)
			if err != nil {
				t.Fatal(err)
			}
			if got.HarvesterVersion != tc.version || got.HasAdvancedPower != tc.advanced {
				t.Errorf("Gather(%q) = %+v, want advanced=%v", tc.version, got, tc.advanced)
			}
		})
	}
}

func TestGatherReportsUnknownWhenTheSettingIsMissing(t *testing.T) {
	got, err := Gather(context.Background(), clientsWith())
	if err == nil {
		t.Fatal("a missing server-version setting must be reported as an error")
	}
	if got.HarvesterVersion != "unknown" || got.HasAdvancedPower {
		t.Errorf("got %+v, want the unknown/false defaults", got)
	}
}

func TestGatherWithoutClients(t *testing.T) {
	got, err := Gather(context.Background(), nil)
	if err == nil || got.HarvesterVersion != "unknown" {
		t.Errorf("got %+v, %v", got, err)
	}
}

// The handler never fails the request: when the version cannot be read it still
// answers 200 with the defaults, so the page keeps loading.
func TestHandler(t *testing.T) {
	t.Run("reads the version", func(t *testing.T) {
		clients := clientsWith(serverVersion("v1.8.2"))
		rr := testutil.Do(Handler(clients), "GET", "/api/v1/capabilities", nil, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d", rr.Code)
		}
		var got map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got["harvesterVersion"] != "v1.8.2" || got["hasAdvancedPower"] != true {
			t.Errorf("body = %v", got)
		}
	})
	t.Run("falls back to defaults", func(t *testing.T) {
		rr := testutil.Do(Handler(clientsWith()), "GET", "/api/v1/capabilities", nil, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d, want 200 even without the setting", rr.Code)
		}
		var got map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got["harvesterVersion"] != "unknown" || got["hasAdvancedPower"] != false {
			t.Errorf("body = %v", got)
		}
	})
}

// Which engines can be used travels with the capabilities, so the UI can build its
// tabs and setup hints from one call.
func TestGatherReportsEachEngine(t *testing.T) {
	t.Setenv("EXPORT_PVC", "")
	t.Setenv("EXPORT_IMAGE", "")
	got, err := Gather(context.Background(), clientsWith(serverVersion("v1.8.2")))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Engines["vmic"].Available {
		t.Errorf("vmic = %+v, want available (its CRD is served)", got.Engines["vmic"])
	}
	if got.Engines["forklift"].Available || got.Engines["forklift"].State != "not-ready" {
		t.Errorf("forklift = %+v, want not-ready (no host provider)", got.Engines["forklift"])
	}
	if got.Engines["export"].State != "disabled" {
		t.Errorf("export = %+v, want disabled", got.Engines["export"])
	}
}

func TestEnginesAreReportedEvenWhenTheVersionCannotBeRead(t *testing.T) {
	got, err := Gather(context.Background(), clientsWith())
	if err == nil {
		t.Fatal("the missing setting must still be reported")
	}
	if len(got.Engines) != 3 {
		t.Errorf("engines = %v, want all three even though the version is unknown", got.Engines)
	}
}

func TestHandlerIncludesTheEngines(t *testing.T) {
	rr := testutil.Do(Handler(clientsWith(serverVersion("v1.8.2"))), "GET", "/api/v1/capabilities", nil, nil)
	var got struct {
		Engines map[string]struct {
			Available bool   `json:"available"`
			State     string `json:"state"`
		} `json:"engines"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Engines["vmic"].State != "available" || got.Engines["forklift"].State == "" {
		t.Errorf("engines = %+v", got.Engines)
	}
}
