// contract_test.go
//
// The VM Import Controller creates its CRDs at runtime from the Go types in its repository, so
// those types are the contract this UI has to stay inside. testdata/upstream-contract.json holds
// every spec/status field path of the four resources at the pinned version, written by
// `go run hack/crd-contract/main.go <checkout> <tag>` (see docs/contract.md). These tests check
// that the typed objects and the handlers only use fields that exist there: a field the
// controller does not know is silently dropped (or rejected) by the API server, and one it
// renamed breaks the UI without any error.
package vmic

import (
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
)

type contractFile struct {
	Version string `json:"version"`
	Kinds   map[string]struct {
		Paths  []string `json:"paths"`
		Opaque []string `json:"opaque"`
	} `json:"kinds"`
}

func loadContract(t *testing.T) contractFile {
	t.Helper()
	raw, err := os.ReadFile("testdata/upstream-contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var c contractFile
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// within reports the paths in got that the contract does not have. A path below an "opaque" one
// (an external type whose inner fields the generator does not enumerate) is not checkable and passes.
func (c contractFile) within(kind string, got map[string]bool) []string {
	k, ok := c.Kinds[kind]
	if !ok {
		return []string{"(unknown kind " + kind + ")"}
	}
	known := map[string]bool{}
	for _, p := range k.Paths {
		known[p] = true
	}
	var bad []string
	for p := range got {
		if known[p] {
			continue
		}
		covered := false
		for _, o := range k.Opaque {
			if strings.HasPrefix(p, o+".") || strings.HasPrefix(p, o+"[]") {
				covered = true
			}
		}
		if !covered {
			bad = append(bad, p)
		}
	}
	sort.Strings(bad)
	return bad
}

// structPaths lists the JSON paths of a Go type, like the generator does for the upstream one.
func structPaths(t reflect.Type, prefix string, out map[string]bool) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || strings.HasPrefix(t.PkgPath(), "k8s.io/apimachinery/pkg/apis/meta/v1") && t.Name() == "Time" {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := strings.Split(f.Tag.Get("json"), ",")
		name := tag[0]
		if name == "-" || (name == "" && !f.Anonymous) {
			continue
		}
		if name == "" { // embedded
			structPaths(f.Type, prefix, out)
			continue
		}
		ft := f.Type
		path := prefix + name
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Slice {
			path += "[]"
			ft = ft.Elem()
			for ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
		}
		out[path] = true
		structPaths(ft, path+".", out)
	}
}

// flatten lists the paths of an object's spec and status, like the generator does.
func flatten(prefix string, v interface{}, out map[string]bool) {
	switch x := v.(type) {
	case map[string]interface{}:
		for k, child := range x {
			// a list is named "k[]", as in the contract and in structPaths
			if _, isList := child.([]interface{}); isList {
				out[prefix+k+"[]"] = true
			} else {
				out[prefix+k] = true
			}
			flatten(prefix+k+".", child, out)
		}
	case []interface{}:
		for _, item := range x {
			if m, ok := item.(map[string]interface{}); ok {
				for k, child := range m {
					p := strings.TrimSuffix(prefix, ".") + "[]." + k
					out[p] = true
					flatten(p+".", child, out)
				}
			}
		}
	}
}

func objectPaths(o *unstructured.Unstructured) map[string]bool {
	out := map[string]bool{}
	for _, root := range []string{"spec", "status"} {
		if v, ok := o.Object[root]; ok {
			out[root] = true
			flatten(root+".", v, out)
		}
	}
	return out
}

func TestContractIsPinnedToTheBundledController(t *testing.T) {
	c := loadContract(t)
	chart, err := os.ReadFile("../../../charts/harvester-migration/Chart.yaml")
	if err != nil {
		t.Fatal(err)
	}
	// "    version: 1.8.2" under the harvester-vm-import-controller dependency
	i := strings.Index(string(chart), "name: harvester-vm-import-controller")
	if i < 0 {
		t.Fatal("dependency not found in Chart.yaml")
	}
	rest := string(chart)[i:]
	j := strings.Index(rest, "version:")
	pinned := strings.Fields(rest[j+len("version:"):])[0]
	if c.Version != "v"+pinned {
		t.Errorf("the contract is for %s but the chart bundles controller %s: regenerate it (docs/contract.md)", c.Version, pinned)
	}
}

func TestLocalTypesStayInsideTheUpstreamContract(t *testing.T) {
	c := loadContract(t)
	got := map[string]bool{}
	structPaths(reflect.TypeOf(VirtualMachineImportSpec{}), "spec.", got)
	structPaths(reflect.TypeOf(VirtualMachineImportStatus{}), "status.", got)
	if bad := c.within("VirtualMachineImport", got); len(bad) > 0 {
		t.Errorf("fields our types declare that %s does not have: %v", c.Version, bad)
	}
}

// Every field the plan editor can send must be a spec field the controller has.
func TestUpdatePlanPayloadFieldsExistUpstream(t *testing.T) {
	c := loadContract(t)
	got := map[string]bool{}
	structPaths(reflect.TypeOf(UpdatePlanPayload{}), "spec.", got)
	if len(got) < 8 {
		t.Fatalf("only %d paths found; the reflection is not seeing the payload", len(got))
	}
	if bad := c.within("VirtualMachineImport", got); len(bad) > 0 {
		t.Errorf("UpdatePlanPayload fields missing upstream: %v", bad)
	}
}

func TestSourceHandlersWriteOnlyKnownFields(t *testing.T) {
	c := loadContract(t)
	created := func(h http.HandlerFunc, body interface{}) map[string]bool {
		rr := testutil.Do(h, http.MethodPost, "/x", body, nil)
		if rr.Code != http.StatusCreated {
			t.Fatalf("status %d: %s", rr.Code, rr.Body)
		}
		var o unstructured.Unstructured
		if err := json.Unmarshal(rr.Body.Bytes(), &o.Object); err != nil {
			t.Fatal(err)
		}
		return objectPaths(&o)
	}
	t.Run("vCenter source", func(t *testing.T) {
		got := created(CreateVmwareSource(emptyClients()), CreateVmwareSourcePayload{Name: "a", Namespace: ns, Endpoint: "https://vc/sdk", Datacenter: "DC0", Username: "u", Password: "p"})
		if len(got) < 4 {
			t.Fatalf("too few paths: %v", got)
		}
		if bad := c.within("VmwareSource", got); len(bad) > 0 {
			t.Errorf("fields written that %s does not have: %v", c.Version, bad)
		}
	})
	t.Run("OVA source", func(t *testing.T) {
		got := created(CreateOvaSource(emptyClients()), CreateOvaSourcePayload{Name: "a", Namespace: ns, URL: "http://f/vm.ova", HttpTimeoutSeconds: 30, Username: "u", Password: "p"})
		if bad := c.within("OvaSource", got); len(bad) > 0 {
			t.Errorf("fields written that %s does not have: %v", c.Version, bad)
		}
	})
	t.Run("OVA source update that adds credentials", func(t *testing.T) {
		cl := withObjects(nil, ovaSourceNoCredentials("a"))
		if rr := testutil.Do(UpdateOvaSource(cl), http.MethodPut, "/x", CreateOvaSourcePayload{URL: "http://f/x.ova", HttpTimeoutSeconds: 5, Username: "u", Password: "p"}, vars); rr.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rr.Code, rr.Body)
		}
		o := getOva(t, cl)
		if bad := c.within("OvaSource", objectPaths(o)); len(bad) > 0 {
			t.Errorf("fields written that %s does not have: %v", c.Version, bad)
		}
	})
}

// The contract tooling itself: structPaths and flatten must agree on shapes, or the checks above prove nothing.
func TestPathHelpersAgree(t *testing.T) {
	type inner struct {
		A string `json:"a"`
	}
	type outer struct {
		X     string  `json:"x"`
		List  []inner `json:"list,omitempty"`
		Ptr   *inner  `json:"ptr,omitempty"`
		Skip  string  `json:"-"`
		Plain string
	}
	got := map[string]bool{}
	structPaths(reflect.TypeOf(outer{}), "spec.", got)
	want := map[string]bool{"spec.x": true, "spec.list[]": true, "spec.list[].a": true, "spec.ptr": true, "spec.ptr.a": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("structPaths = %v, want %v", got, want)
	}
	o := &unstructured.Unstructured{Object: map[string]interface{}{"spec": map[string]interface{}{
		"x": "1", "list": []interface{}{map[string]interface{}{"a": "2"}}, "ptr": map[string]interface{}{"a": "3"},
	}}}
	if flat := objectPaths(o); !reflect.DeepEqual(flat, map[string]bool{"spec": true, "spec.x": true, "spec.list[]": true, "spec.list[].a": true, "spec.ptr": true, "spec.ptr.a": true}) {
		t.Errorf("flatten = %v", flat)
	}
}

var _ runtime.Object = (*unstructured.Unstructured)(nil)
