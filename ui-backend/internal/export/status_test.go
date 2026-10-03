// status_test.go
package export

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"
	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// Handler-level tests for the export API. They cover what the handlers promise (never
// export a running VM, refuse when export is not configured, honour the concurrency
// cap) and how a failing Kubernetes call maps to an HTTP status: the API server's own
// status passes through, and 500 is kept for failures that are the server's.

const labs = "labs"

func exportEnv(t *testing.T) {
	t.Helper()
	t.Setenv("EXPORT_PVC", "exports")
	t.Setenv("EXPORT_IMAGE", "img:1")
	t.Setenv("EXPORT_STORAGE_SIZE", "10Gi")
	t.Setenv("EXPORT_STORAGE_CLASS", "sc")
	t.Setenv("EXPORT_ROOT", "")
	t.Setenv("EXPORT_MAX_CONCURRENT", "2")
}

func vmObj(name string, withDisk bool) *unstructured.Unstructured {
	tmpl := map[string]interface{}{"domain": map[string]interface{}{
		"cpu": map[string]interface{}{"cores": int64(2)}, "memory": map[string]interface{}{"guest": "2Gi"},
		"devices": map[string]interface{}{"disks": []interface{}{}},
	}}
	if withDisk {
		dev := tmpl["domain"].(map[string]interface{})["devices"].(map[string]interface{})
		dev["disks"] = []interface{}{map[string]interface{}{"name": "rootdisk", "bootOrder": int64(1), "disk": map[string]interface{}{"bus": "virtio"}}}
		tmpl["volumes"] = []interface{}{map[string]interface{}{"name": "rootdisk", "persistentVolumeClaim": map[string]interface{}{"claimName": "root-pvc"}}}
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kubevirt.io/v1", "kind": "VirtualMachine",
		"metadata": map[string]interface{}{"name": name, "namespace": labs},
		"spec":     map[string]interface{}{"runStrategy": "Halted", "template": map[string]interface{}{"spec": tmpl}},
	}}
}

func vmiObj(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kubevirt.io/v1", "kind": "VirtualMachineInstance",
		"metadata": map[string]interface{}{"name": name, "namespace": labs},
	}}
}

func rootPVC() *corev1.PersistentVolumeClaim {
	block := corev1.PersistentVolumeBlock
	sc := "sc"
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "root-pvc", Namespace: labs},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeMode: &block, StorageClassName: &sc,
			Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("50Gi")}},
		},
	}
}

type jobWithID struct {
	Job *batchv1.Job
	ID  string
}

// exportJob is a realistic export Job, as the Create handler would have made it.
func exportJob(t *testing.T) *jobWithID {
	t.Helper()
	spec := testExportSpec()
	job, err := buildExportJob(spec, ExportJobOptions{Namespace: labs, Image: "img:1", ExportPVC: "exports", SourceClaims: []string{"d1"}})
	if err != nil {
		t.Fatal(err)
	}
	return &jobWithID{Job: job, ID: spec.ExportID}
}

func clientsFor(core []runtime.Object, dyn ...runtime.Object) *kube.Clients {
	return testutil.NewClientsWithDynamic(core, dyn...)
}

func failing(verb, resource string, err error, core []runtime.Object, dyn ...runtime.Object) *kube.Clients {
	c := clientsFor(core, dyn...)
	testutil.Fail(c, verb, resource, err)
	return c
}

func listJobs(t *testing.T, c *kube.Clients) int {
	t.Helper()
	l, err := c.Clientset.BatchV1().Jobs(labs).List(t.Context(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return len(l.Items)
}

var createReq = CreateExportRequest{Namespace: labs, Name: "vm1"}

func TestCreate(t *testing.T) {
	exportEnv(t)
	good := []runtime.Object{rootPVC()}

	t.Run("creates the export Job", func(t *testing.T) {
		c := clientsFor(good, vmObj("vm1", true))
		rr := testutil.Do(Create(c), "POST", "/x", createReq, nil)
		// 202: the work happens asynchronously in the Job.
		if rr.Code != http.StatusAccepted {
			t.Fatalf("status %d: %s", rr.Code, rr.Body)
		}
		var resp map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if resp["exportId"] == "" || resp["jobName"] != "vm-export-"+resp["exportId"] || resp["namespace"] != labs || resp["profile"] != "vmware" {
			t.Errorf("response = %v", resp)
		}
		if n := listJobs(t, c); n != 1 {
			t.Errorf("%d Jobs created, want 1", n)
		}
		// The export volume is provisioned in the VM's namespace on first use.
		if _, err := c.Clientset.CoreV1().PersistentVolumeClaims(labs).Get(t.Context(), "exports", metav1.GetOptions{}); err != nil {
			t.Errorf("export volume not created: %v", err)
		}
	})

	// The load-bearing safety property: reading an attached disk yields a torn image.
	t.Run("a running VM is refused and nothing is created", func(t *testing.T) {
		c := clientsFor(good, vmObj("vm1", true), vmiObj("vm1"))
		rr := testutil.Do(Create(c), "POST", "/x", createReq, nil)
		if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "VM is running") {
			t.Fatalf("status %d: %s", rr.Code, rr.Body)
		}
		if n := listJobs(t, c); n != 0 {
			t.Errorf("%d Jobs created for a running VM", n)
		}
	})

	t.Run("is refused when export is not configured", func(t *testing.T) {
		t.Setenv("EXPORT_PVC", "")
		rr := testutil.Do(Create(clientsFor(good, vmObj("vm1", true))), "POST", "/x", createReq, nil)
		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("status %d, want 503", rr.Code)
		}
	})

	t.Run("honours the concurrency cap", func(t *testing.T) {
		t.Setenv("EXPORT_MAX_CONCURRENT", "1")
		running := exportJob(t).Job // no status: counts as running
		c := clientsFor(append([]runtime.Object{running}, good...), vmObj("vm1", true))
		if rr := testutil.Do(Create(c), "POST", "/x", createReq, nil); rr.Code != http.StatusTooManyRequests {
			t.Errorf("status %d, want 429", rr.Code)
		}
	})

	testutil.Run(t, []testutil.Row{
		testutil.Case("malformed body is 400", Create(clientsFor(good)), "POST", "{nope", nil, http.StatusBadRequest),
		testutil.Case("missing name is 400", Create(clientsFor(good)), "POST", CreateExportRequest{Namespace: labs}, nil, http.StatusBadRequest),
		testutil.Case("unknown profile is 400", Create(clientsFor(good)), "POST", CreateExportRequest{Namespace: labs, Name: "vm1", Profile: "nope"}, nil, http.StatusBadRequest),
		testutil.Case("VM without PVC-backed disks is 422", Create(clientsFor(good, vmObj("vm1", false))), "POST", createReq, nil, http.StatusUnprocessableEntity),
		testutil.Case("a disk whose capacity is unknown is 422", Create(clientsFor(nil, vmObj("vm1", true))), "POST", createReq, nil, http.StatusUnprocessableEntity),

		// API failures keep the API server's status.
		testutil.Case("missing VM is 404", Create(clientsFor(good)), "POST", createReq, nil, http.StatusNotFound),
		testutil.Case("VM lookup forbidden is 403", Create(failing("get", "virtualmachines", testutil.ErrForbidden(), good, vmObj("vm1", true))), "POST", createReq, nil, http.StatusForbidden),
		testutil.Case("listing exports forbidden is 403", Create(failing("list", "jobs", testutil.ErrForbidden(), good, vmObj("vm1", true))), "POST", createReq, nil, http.StatusForbidden),
		testutil.Case("power-state lookup forbidden is 403", Create(failing("get", "virtualmachineinstances", testutil.ErrForbidden(), good, vmObj("vm1", true))), "POST", createReq, nil, http.StatusForbidden),
		testutil.Case("creating the export volume forbidden is 403", Create(failing("create", "persistentvolumeclaims", testutil.ErrForbidden(), good, vmObj("vm1", true))), "POST", createReq, nil, http.StatusForbidden),
		testutil.Case("creating the Job: already exists is 409", Create(failing("create", "jobs", testutil.ErrAlreadyExists(), good, vmObj("vm1", true))), "POST", createReq, nil, http.StatusConflict),
		testutil.Case("creating the Job forbidden is 403", Create(failing("create", "jobs", testutil.ErrForbidden(), good, vmObj("vm1", true))), "POST", createReq, nil, http.StatusForbidden),
	})
}

func TestPreview(t *testing.T) {
	exportEnv(t)
	good := []runtime.Object{rootPVC()}
	body := map[string]string{"namespace": labs, "name": "vm1"}
	testutil.Run(t, []testutil.Row{
		testutil.Case("malformed body is 400", Preview(clientsFor(good)), "POST", "{nope", nil, http.StatusBadRequest),
		testutil.Case("missing VM is 404", Preview(clientsFor(good)), "POST", body, nil, http.StatusNotFound),
		testutil.Case("VM lookup forbidden is 403", Preview(failing("get", "virtualmachines", testutil.ErrForbidden(), good, vmObj("vm1", true))), "POST", body, nil, http.StatusForbidden),
	})
}

func TestListGetDelete(t *testing.T) {
	exportEnv(t)
	job := exportJob(t)
	vars := map[string]string{"namespace": labs, "id": job.ID}
	missing := map[string]string{"namespace": labs, "id": "nope"}
	seeded := func() *kube.Clients { return clientsFor([]runtime.Object{job.Job}) }

	t.Run("lists and fetches a seeded export", func(t *testing.T) {
		rr := testutil.Do(List(seeded()), "GET", "/x", nil, nil)
		if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), job.ID) {
			t.Errorf("list: %d %s", rr.Code, rr.Body)
		}
		rr = testutil.Do(Get(seeded()), "GET", "/x", nil, vars)
		if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), job.ID) {
			t.Errorf("get: %d %s", rr.Code, rr.Body)
		}
	})
	t.Run("deleting removes the Job", func(t *testing.T) {
		c := seeded()
		if rr := testutil.Do(Delete(c), "DELETE", "/x", nil, vars); rr.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rr.Code, rr.Body)
		}
		if n := listJobs(t, c); n != 0 {
			t.Errorf("%d Jobs left", n)
		}
	})

	testutil.Run(t, []testutil.Row{
		testutil.Case("list: forbidden is 403", List(failing("list", "jobs", testutil.ErrForbidden(), nil)), "GET", nil, nil, http.StatusForbidden),
		testutil.Case("get: missing is 404", Get(seeded()), "GET", nil, missing, http.StatusNotFound),
		testutil.Case("get: forbidden is 403", Get(failing("get", "jobs", testutil.ErrForbidden(), []runtime.Object{job.Job})), "GET", nil, vars, http.StatusForbidden),
		testutil.Case("delete: missing is 404", Delete(seeded()), "DELETE", nil, missing, http.StatusNotFound),
		testutil.Case("delete: lookup forbidden is 403", Delete(failing("get", "jobs", testutil.ErrForbidden(), []runtime.Object{job.Job})), "DELETE", nil, vars, http.StatusForbidden),
		testutil.Case("delete: forbidden is 403", Delete(failing("delete", "jobs", testutil.ErrForbidden(), []runtime.Object{job.Job})), "DELETE", nil, vars, http.StatusForbidden),
	})

	// Purging files from a volume this pod does not mount goes through a cleanup Job;
	// if that Job cannot be created the export is kept, with the API's status.
	t.Run("a purge whose cleanup Job is forbidden is 403 and keeps the export", func(t *testing.T) {
		c := failing("create", "jobs", testutil.ErrForbidden(), []runtime.Object{job.Job})
		rr := testutil.Do(Delete(c), "DELETE", "/x?purge=true", nil, vars)
		if rr.Code != http.StatusForbidden {
			t.Errorf("status %d: %s", rr.Code, rr.Body)
		}
		if n := listJobs(t, c); n != 1 {
			t.Errorf("the export must be kept; %d Jobs left", n)
		}
	})
}

func TestLogs(t *testing.T) {
	exportEnv(t)
	job := exportJob(t)
	vars := map[string]string{"namespace": labs, "id": job.ID}

	testutil.Run(t, []testutil.Row{
		testutil.Case("logs: no pod is 404", GetLogs(clientsFor(nil)), "GET", nil, vars, http.StatusNotFound),
		testutil.Case("logs: pod list forbidden is 403", GetLogs(failing("list", "pods", testutil.ErrForbidden(), nil)), "GET", nil, vars, http.StatusForbidden),
	})
}
