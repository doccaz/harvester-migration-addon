// engines_test.go
package engines

import (
	"context"
	"errors"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"
	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
)

// What the API server answers for a resource whose CRD is not installed: a 404
// that names no object (an object that is merely missing is named in the status).
func noSuchResource() error {
	return apierrors.NewGenericServerResponse(404, "get", schema.GroupResource{Group: "forklift.konveyor.io", Resource: "providers"}, "",
		"the server could not find the requested resource", 0, false)
}

func hostProvider(ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "forklift.konveyor.io/v1beta1", "kind": "Provider",
		"metadata": map[string]interface{}{"name": "host", "namespace": ns},
		"spec":     map[string]interface{}{"type": "openshift"},
	}}
}

func TestForklift(t *testing.T) {
	boom := errors.New("etcdserver: request timed out")
	cases := []struct {
		name      string
		clients   func() *kube.Clients
		wantState State
		wantAvail bool
		msg       string // a substring the message must contain
	}{
		{"host provider present", func() *kube.Clients { return testutil.NewClientsWithDynamic(nil, hostProvider("forklift")) }, StateAvailable, true, ""},
		{"installed, host provider missing", func() *kube.Clients { return testutil.NewClientsWithDynamic(nil) }, StateNotReady, false, "host Provider"},
		{"host provider only in another namespace", func() *kube.Clients { return testutil.NewClientsWithDynamic(nil, hostProvider("other")) }, StateNotReady, false, "forklift"},
		{"CRD not installed", func() *kube.Clients {
			c := testutil.NewClientsWithDynamic(nil)
			testutil.Fail(c, "get", "providers", noSuchResource())
			return c
		}, StateNotInstalled, false, "not installed"},
		{"no kind match", func() *kube.Clients {
			c := testutil.NewClientsWithDynamic(nil)
			testutil.Fail(c, "get", "providers", &meta.NoKindMatchError{GroupKind: schema.GroupKind{Group: "forklift.konveyor.io", Kind: "Provider"}})
			return c
		}, StateNotInstalled, false, "not installed"},
		{"forbidden is not 'absent'", func() *kube.Clients {
			c := testutil.NewClientsWithDynamic(nil, hostProvider("forklift"))
			testutil.Fail(c, "get", "providers", testutil.ErrForbidden())
			return c
		}, StateForbidden, false, "permission"},
		{"unexpected error is unknown, with its text", func() *kube.Clients {
			c := testutil.NewClientsWithDynamic(nil, hostProvider("forklift"))
			testutil.Fail(c, "get", "providers", boom)
			return c
		}, StateUnknown, false, "timed out"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Forklift(context.Background(), tc.clients(), "forklift")
			if got.State != tc.wantState || got.Available != tc.wantAvail {
				t.Fatalf("got state %q available %v, want %q %v (%s)", got.State, got.Available, tc.wantState, tc.wantAvail, got.Message)
			}
			if got.Namespace != "forklift" {
				t.Errorf("namespace = %q", got.Namespace)
			}
			if tc.msg != "" && !strings.Contains(got.Message, tc.msg) {
				t.Errorf("message %q lacks %q", got.Message, tc.msg)
			}
			if tc.wantAvail && got.Message != "" {
				t.Errorf("an available engine has no message, got %q", got.Message)
			}
		})
	}
}

func TestForkliftDefaultsToTheForkliftNamespace(t *testing.T) {
	got := Forklift(context.Background(), testutil.NewClientsWithDynamic(nil, hostProvider("forklift")), "")
	if !got.Available || got.Namespace != "forklift" {
		t.Errorf("got %+v", got)
	}
}

func vmicClients() *kube.Clients {
	return testutil.NewClientsWithListKinds(map[schema.GroupVersionResource]string{kube.VMIGVR: "VirtualMachineImportList"})
}

func TestVMIC(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		wantState State
	}{
		{"served", nil, StateAvailable},
		{"CRD not installed", noSuchResource(), StateNotInstalled},
		{"forbidden", testutil.ErrForbidden(), StateForbidden},
		{"unexpected", errors.New("connection refused"), StateUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := vmicClients()
			if tc.err != nil {
				testutil.Fail(c, "list", "virtualmachineimports", tc.err)
			}
			got := VMIC(context.Background(), c)
			if got.State != tc.wantState || got.Available != (tc.wantState == StateAvailable) {
				t.Fatalf("got %q available %v, want %q (%s)", got.State, got.Available, tc.wantState, got.Message)
			}
			if tc.wantState != StateAvailable && got.Message == "" {
				t.Error("an unavailable engine must say why")
			}
		})
	}
}

func TestExport(t *testing.T) {
	t.Setenv("EXPORT_PVC", "")
	t.Setenv("EXPORT_IMAGE", "")
	if got := Export(); got.Available || got.State != StateDisabled || !strings.Contains(got.Message, "export.enabled") {
		t.Errorf("unconfigured: %+v", got)
	}
	t.Setenv("EXPORT_PVC", "exports")
	if got := Export(); got.Available {
		t.Errorf("a claim without an image is not enough: %+v", got)
	}
	t.Setenv("EXPORT_IMAGE", "img:1")
	if got := Export(); !got.Available || got.State != StateAvailable {
		t.Errorf("configured: %+v", got)
	}
}

func TestAllNamesEveryEngine(t *testing.T) {
	c := testutil.NewClientsWithListKinds(map[schema.GroupVersionResource]string{kube.VMIGVR: "VirtualMachineImportList"})
	got := All(context.Background(), c)
	for _, k := range []string{"vmic", "forklift", "export"} {
		if _, ok := got[k]; !ok {
			t.Errorf("engine %q missing from %v", k, got)
		}
	}
}

var _ runtime.Object = (*unstructured.Unstructured)(nil)
