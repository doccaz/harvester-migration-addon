// contract_test.go
package export

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Export and cleanup run as Kubernetes Jobs that execute this same binary
// ("<BinaryPath> <arg>"), and main() dispatches on those arguments. If the Job
// command, the dispatch strings and the image layout drift apart, exports fail only
// at run time on a cluster, so the contract is pinned here.
func TestJobsInvokeTheBinaryTheWayMainDispatches(t *testing.T) {
	worker, err := buildExportJob(testExportSpec(), ExportJobOptions{
		Namespace: "labs", Image: "img:1", ExportPVC: "exports", SourceClaims: []string{"d1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cleanup, err := buildExportCleanupJob(ExportCleanupJobOptions{
		Namespace: "labs", Image: "img:1", ExportPVC: "exports", ExportID: "abc123", TargetName: "mine",
	})
	if err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		got  []string
		want string
	}{
		"worker":  {worker.Spec.Template.Spec.Containers[0].Command, WorkerArg},
		"cleanup": {cleanup.Spec.Template.Spec.Containers[0].Command, CleanupArg},
	} {
		if len(tc.got) != 2 || tc.got[0] != BinaryPath || tc.got[1] != tc.want {
			t.Errorf("%s Job command = %v, want [%s %s]", name, tc.got, BinaryPath, tc.want)
		}
	}
	if WorkerArg == CleanupArg || WorkerArg == "" || CleanupArg == "" {
		t.Errorf("the mode arguments must be distinct and non-empty: %q %q", WorkerArg, CleanupArg)
	}
}

// The Jobs run BinaryPath inside the same image as the API, so the Dockerfile must
// install the program there.
func TestTheImageInstallsTheBinaryWhereTheJobsRunIt(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	dockerfile := filepath.Join(filepath.Dir(here), "..", "..", "..", "Dockerfile")
	b, err := os.ReadFile(dockerfile)
	if err != nil {
		t.Skipf("Dockerfile not found (%v); run from a full checkout", err)
	}
	if !strings.Contains(string(b), " "+BinaryPath+"\n") {
		t.Errorf("Dockerfile does not COPY the binary to %s", BinaryPath)
	}
}
