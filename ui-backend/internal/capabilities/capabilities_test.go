// capabilities_test.go
package capabilities

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

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
			clients := testutil.NewClientsWithDynamic(nil, []runtime.Object{serverVersion(tc.version)}...)
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
	got, err := Gather(context.Background(), testutil.NewClients())
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
		clients := testutil.NewClientsWithDynamic(nil, serverVersion("v1.8.2"))
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
		rr := testutil.Do(Handler(testutil.NewClients()), "GET", "/api/v1/capabilities", nil, nil)
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
