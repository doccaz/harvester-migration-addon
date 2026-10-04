// pvc_failure_test.go
package export

import (
	"net/http"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
)

// When the PVCs cannot be LISTED the cause is the caller's permissions or the API, not
// a missing claim, and the answer must say so with the API's own status instead of the
// 422 "check that its PersistentVolumeClaim exists" that suits a claim that is absent.
func TestExportSaysWhyDisksCannotBeRead(t *testing.T) {
	exportEnv(t)
	good := []runtime.Object{rootPVC()}
	req := CreateExportRequest{Namespace: labs, Name: "vm1", Profile: "vmware"}

	t.Run("preview works when the claims can be read", func(t *testing.T) {
		rr := testutil.Do(Preview(clientsFor(good, vmObj("vm1", true))), "POST", "/x", req, nil)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rr.Code, rr.Body)
		}
	})
	t.Run("preview: PVC list forbidden is 403 and names the claims", func(t *testing.T) {
		rr := testutil.Do(Preview(failing("list", "persistentvolumeclaims", testutil.ErrForbidden(), good, vmObj("vm1", true))), "POST", "/x", req, nil)
		if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "PersistentVolumeClaims") {
			t.Errorf("status %d: %s", rr.Code, rr.Body)
		}
	})
	t.Run("create: PVC list forbidden is 403, not the 422 for an absent claim", func(t *testing.T) {
		rr := testutil.Do(Create(failing("list", "persistentvolumeclaims", testutil.ErrForbidden(), good, vmObj("vm1", true))), "POST", "/x", req, nil)
		if rr.Code != http.StatusForbidden {
			t.Errorf("status %d: %s", rr.Code, rr.Body)
		}
	})
}
