// download_test.go
package export

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
)

var testKey = []byte("0123456789abcdef0123456789abcdef")

func TestTicketRoundTripAndRejections(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	c := ticketClaims{Namespace: "labs", ID: "abc123", IP: "10.42.0.7", Expires: now.Add(time.Minute).Unix()}
	good := signTicket(testKey, c)

	if ip, err := verifyTicket(testKey, good, "labs", "abc123", now); err != nil || ip != "10.42.0.7" {
		t.Fatalf("valid ticket: ip %q, err %v", ip, err)
	}

	p, sig, _ := strings.Cut(good, ".")
	flip := func(s string) string { // change one character
		b := []byte(s)
		if b[3] == 'A' {
			b[3] = 'B'
		} else {
			b[3] = 'A'
		}
		return string(b)
	}
	other := signTicket(testKey, ticketClaims{Namespace: "labs", ID: "abc123", IP: "10.9.9.9", Expires: c.Expires})
	otherP, _, _ := strings.Cut(other, ".")

	for name, tc := range map[string]struct {
		ticket, ns, id string
		now            time.Time
		key            []byte
		want           error
	}{
		"empty":              {"", "labs", "abc123", now, testKey, errBadTicket},
		"no separator":       {p, "labs", "abc123", now, testKey, errBadTicket},
		"garbage":            {"not.a.ticket", "labs", "abc123", now, testKey, errBadTicket},
		"tampered claims":    {flip(p) + "." + sig, "labs", "abc123", now, testKey, errBadTicket},
		"tampered sig":       {p + "." + flip(sig), "labs", "abc123", now, testKey, errBadTicket},
		"claims swapped":     {otherP + "." + sig, "labs", "abc123", now, testKey, errBadTicket},
		"other namespace":    {good, "default", "abc123", now, testKey, errBadTicket},
		"other export":       {good, "labs", "def456", now, testKey, errBadTicket},
		"wrong key":          {good, "labs", "abc123", now, []byte("another-key-another-key-another"), errBadTicket},
		"expired":            {good, "labs", "abc123", now.Add(time.Minute), testKey, errExpiredTicket},
		"long expired":       {good, "labs", "abc123", now.Add(time.Hour), testKey, errExpiredTicket},
		"signed bad ip":      {signTicket(testKey, ticketClaims{Namespace: "labs", ID: "abc123", IP: "not-an-ip", Expires: c.Expires}), "labs", "abc123", now, testKey, errBadTicket},
		"signed empty ip":    {signTicket(testKey, ticketClaims{Namespace: "labs", ID: "abc123", Expires: c.Expires}), "labs", "abc123", now, testKey, errBadTicket},
		"no expiry (zero)":   {signTicket(testKey, ticketClaims{Namespace: "labs", ID: "abc123", IP: "10.0.0.1"}), "labs", "abc123", now, testKey, errExpiredTicket},
		"expiry at the tick": {signTicket(testKey, ticketClaims{Namespace: "labs", ID: "abc123", IP: "10.0.0.1", Expires: now.Unix()}), "labs", "abc123", now, testKey, errExpiredTicket},
	} {
		if _, err := verifyTicket(tc.key, tc.ticket, tc.ns, tc.id, tc.now); err != tc.want {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
}

func TestServeTokenIsPerExportAndPerKey(t *testing.T) {
	a := serveToken(testKey, "labs", "abc")
	if a == serveToken(testKey, "labs", "abd") || a == serveToken(testKey, "other", "abc") ||
		a == serveToken([]byte("another-key-another-key-another"), "labs", "abc") {
		t.Fatal("serve token must differ per export, namespace and key")
	}
	if a != serveToken(testKey, "labs", "abc") {
		t.Fatal("serve token must be stable")
	}
	if len(a) < 32 {
		t.Fatalf("token too short: %q", a)
	}
}

// finishedJob is the fixture export Job, marked succeeded and uid'd.
func finishedJob(t *testing.T) *batchv1.Job {
	t.Helper()
	j := exportJob(t).Job
	j.UID = types.UID("job-uid-1")
	j.Status.Succeeded = 1
	return j
}

func testCfg() exportConfig {
	return exportConfig{Image: "img:1", PVC: "exports"}
}

func TestBuildServePod(t *testing.T) {
	job := finishedJob(t)
	pod, err := buildServePod(job, testCfg(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	id := job.Labels[exportLabelID]
	if pod.Name != "vm-export-serve-"+id || pod.Namespace != labs {
		t.Errorf("name/namespace = %s/%s", pod.Namespace, pod.Name)
	}
	if _, ok := pod.Labels[exportLabelMarker]; ok {
		t.Error("a serve pod must not carry the export marker label (it would be listed as an export)")
	}
	if pod.Labels[exportLabelServe] != id {
		t.Error("serve label missing")
	}
	if len(pod.OwnerReferences) != 1 || pod.OwnerReferences[0].Kind != "Job" ||
		pod.OwnerReferences[0].Name != job.Name || pod.OwnerReferences[0].UID != "job-uid-1" {
		t.Errorf("owner = %+v, want the export Job", pod.OwnerReferences)
	}
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken {
		t.Error("the serve pod needs no API access")
	}
	if pod.Spec.ActiveDeadlineSeconds == nil || *pod.Spec.ActiveDeadlineSeconds <= 0 {
		t.Error("the serve pod must have a deadline")
	}
	if v := pod.Spec.Volumes[0].PersistentVolumeClaim; v == nil || v.ClaimName != "exports" || !v.ReadOnly {
		t.Errorf("volume = %+v, want the export claim, read-only", pod.Spec.Volumes[0])
	}
	c := pod.Spec.Containers[0]
	if !c.VolumeMounts[0].ReadOnly {
		t.Error("the export volume must be mounted read-only")
	}
	if strings.Join(c.Command, " ") != BinaryPath+" "+ServeArg || c.Image != "img:1" {
		t.Errorf("command/image = %v / %s", c.Command, c.Image)
	}
	env := map[string]string{}
	for _, e := range c.Env {
		env[e.Name] = e.Value
	}
	if env["EXPORT_SERVE_TOKEN"] != "tok" || env["EXPORT_TARGET"] != job.Annotations[exportAnnTargetName] || env["EXPORT_ROOT"] != exportMountPath {
		t.Errorf("env = %v", env)
	}
}

func TestBuildServePodRefusesBadInput(t *testing.T) {
	good := finishedJob(t)
	mod := func(f func(j *batchv1.Job, c *exportConfig)) error {
		j, c := good.DeepCopy(), testCfg()
		f(j, &c)
		_, err := buildServePod(j, c, "tok")
		return err
	}
	for name, f := range map[string]func(*batchv1.Job, *exportConfig){
		"no export id":    func(j *batchv1.Job, c *exportConfig) { delete(j.Labels, exportLabelID) },
		"bad export id":   func(j *batchv1.Job, c *exportConfig) { j.Labels[exportLabelID] = "../x" },
		"no target":       func(j *batchv1.Job, c *exportConfig) { delete(j.Annotations, exportAnnTargetName) },
		"escaping target": func(j *batchv1.Job, c *exportConfig) { j.Annotations[exportAnnTargetName] = "../../etc/x" },
		"no image":        func(j *batchv1.Job, c *exportConfig) { c.Image = "" },
		"no storage":      func(j *batchv1.Job, c *exportConfig) { c.PVC = ""; j.Spec.Template.Spec.Volumes = nil },
	} {
		if mod(f) == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func podIn(job *batchv1.Job, phase corev1.PodPhase, ip string, ready bool) *corev1.Pod {
	p, _ := buildServePod(job, testCfg(), "tok")
	p.Status.Phase = phase
	p.Status.PodIP = ip
	st := corev1.ConditionFalse
	if ready {
		st = corev1.ConditionTrue
	}
	p.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: st}}
	return p
}

func ticketVars(job *batchv1.Job) map[string]string {
	return map[string]string{"namespace": job.Namespace, "id": job.Labels[exportLabelID]}
}

func TestDownloadTicket(t *testing.T) {
	exportEnv(t)
	job := finishedJob(t)
	vars := ticketVars(job)
	id := vars["id"]

	t.Run("an unfinished export has nothing to download", func(t *testing.T) {
		running := job.DeepCopy()
		running.Status.Succeeded = 0
		running.Status.Active = 1
		rr := testutil.Do(DownloadTicket(clientsFor([]runtime.Object{running})), "POST", "/x", nil, vars)
		if rr.Code != http.StatusConflict {
			t.Errorf("status %d, want 409", rr.Code)
		}
	})
	t.Run("missing export is 404, forbidden is 403", func(t *testing.T) {
		testutil.Run(t, []testutil.Row{
			testutil.Case("missing", DownloadTicket(clientsFor(nil)), "POST", nil, vars, http.StatusNotFound),
			testutil.Case("forbidden", DownloadTicket(failing("get", "jobs", testutil.ErrForbidden(), []runtime.Object{job})), "POST", nil, vars, http.StatusForbidden),
		})
	})
	t.Run("first call creates the serve pod and answers 202", func(t *testing.T) {
		c := clientsFor([]runtime.Object{job})
		rr := testutil.Do(DownloadTicket(c), "POST", "/x", nil, vars)
		if rr.Code != http.StatusAccepted || !strings.Contains(rr.Body.String(), `"starting"`) {
			t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
		}
		pod, err := c.Clientset.CoreV1().Pods(labs).Get(t.Context(), exportServePodName(id), metav1.GetOptions{})
		if err != nil {
			t.Fatalf("serve pod not created: %v", err)
		}
		var tok string
		for _, e := range pod.Spec.Containers[0].Env {
			if e.Name == "EXPORT_SERVE_TOKEN" {
				tok = e.Value
			}
		}
		if tok != serveToken(ticketKey(), labs, id) {
			t.Error("the pod must expect the token the proxy will derive")
		}
	})
	t.Run("a user who cannot create pods gets Kubernetes' refusal", func(t *testing.T) {
		rr := testutil.Do(DownloadTicket(failing("create", "pods", testutil.ErrForbidden(), []runtime.Object{job})), "POST", "/x", nil, vars)
		if rr.Code != http.StatusForbidden {
			t.Errorf("status %d, want 403", rr.Code)
		}
	})
	t.Run("a pod that is not ready yet keeps answering 202", func(t *testing.T) {
		for name, p := range map[string]*corev1.Pod{
			"pending":       podIn(job, corev1.PodPending, "", false),
			"running no ip": podIn(job, corev1.PodRunning, "", true),
			"not ready":     podIn(job, corev1.PodRunning, "10.42.0.7", false),
		} {
			rr := testutil.Do(DownloadTicket(clientsFor([]runtime.Object{job, p})), "POST", "/x", nil, vars)
			if rr.Code != http.StatusAccepted {
				t.Errorf("%s: status %d, want 202", name, rr.Code)
			}
		}
	})
	t.Run("a pod that exited is removed so the next call recreates it", func(t *testing.T) {
		for _, ph := range []corev1.PodPhase{corev1.PodSucceeded, corev1.PodFailed} {
			c := clientsFor([]runtime.Object{job, podIn(job, ph, "10.42.0.7", true)})
			rr := testutil.Do(DownloadTicket(c), "POST", "/x", nil, vars)
			if rr.Code != http.StatusAccepted {
				t.Errorf("%s: status %d, want 202", ph, rr.Code)
			}
			if _, err := c.Clientset.CoreV1().Pods(labs).Get(t.Context(), exportServePodName(id), metav1.GetOptions{}); err == nil {
				t.Errorf("%s: the exited pod must be deleted", ph)
			}
		}
	})
	t.Run("a ready pod yields a ticket for exactly this export and pod", func(t *testing.T) {
		c := clientsFor([]runtime.Object{job, podIn(job, corev1.PodRunning, "10.42.0.7", true)})
		rr := testutil.Do(DownloadTicket(c), "POST", "/x", nil, vars)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
		}
		var out struct{ State, URL, FileName, ExpiresAt string }
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.State != "ready" || out.FileName != job.Annotations[exportAnnTargetName]+".ova" {
			t.Errorf("response = %+v", out)
		}
		u, err := url.Parse(out.URL)
		if err != nil || u.Path != "/api/v1/exports/labs/"+id+"/download" {
			t.Fatalf("url = %q", out.URL)
		}
		ip, err := verifyTicket(ticketKey(), u.Query().Get("ticket"), labs, id, time.Now())
		if err != nil || ip != "10.42.0.7" {
			t.Errorf("ticket: ip %q, err %v", ip, err)
		}
		if _, err := verifyTicket(ticketKey(), u.Query().Get("ticket"), labs, id, time.Now().Add(ticketTTL+time.Second)); err != errExpiredTicket {
			t.Errorf("the ticket must expire after %s, got %v", ticketTTL, err)
		}
	})
}

// fakeServePod stands in for the serve pod: it checks the bearer token and the
// path, and serves content with Range support.
func fakeServePod(t *testing.T, token string, content string) (*httptest.Server, *[]http.Header) {
	t.Helper()
	var seen []http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Clone())
		if r.URL.Path != ServePath || r.URL.RawQuery != "" {
			http.Error(w, "unexpected "+r.URL.String(), http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="vm.ova"`)
		http.ServeContent(w, r, "vm.ova", time.Unix(1, 0), strings.NewReader(content))
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func proxyServer(t *testing.T, upstream *httptest.Server) *httptest.Server {
	t.Helper()
	old := serveURL
	serveURL = func(string) *url.URL {
		u, _ := url.Parse(upstream.URL + ServePath)
		return u
	}
	t.Cleanup(func() { serveURL = old })
	r := mux.NewRouter()
	r.HandleFunc("/api/v1/exports/{namespace}/{id}/download", DownloadProxy()).Methods("GET")
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func TestDownloadProxy(t *testing.T) {
	const content = "0123456789abcdefghijklmnopqrstuvwxyz"
	key := ticketKey()
	token := serveToken(key, labs, "abc123")
	up, seen := fakeServePod(t, token, content)
	px := proxyServer(t, up)

	mint := func(ns, id string, ttl time.Duration) string {
		return signTicket(key, ticketClaims{Namespace: ns, ID: id, IP: "10.42.0.7", Expires: time.Now().Add(ttl).Unix()})
	}
	get := func(path string, hdr map[string]string) (*http.Response, string) {
		req, _ := http.NewRequest("GET", px.URL+path, nil)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}
	base := "/api/v1/exports/labs/abc123/download"

	t.Run("streams the whole file with its headers", func(t *testing.T) {
		resp, body := get(base+"?ticket="+url.QueryEscape(mint(labs, "abc123", time.Minute)), nil)
		if resp.StatusCode != 200 || body != content {
			t.Fatalf("status %d, body %q", resp.StatusCode, body)
		}
		if resp.Header.Get("Content-Disposition") != `attachment; filename="vm.ova"` || resp.Header.Get("Accept-Ranges") != "bytes" {
			t.Errorf("headers = %v", resp.Header)
		}
	})
	t.Run("passes Range through so downloads resume", func(t *testing.T) {
		resp, body := get(base+"?ticket="+url.QueryEscape(mint(labs, "abc123", time.Minute)), map[string]string{"Range": "bytes=10-19"})
		if resp.StatusCode != http.StatusPartialContent || body != content[10:20] {
			t.Errorf("status %d, body %q", resp.StatusCode, body)
		}
	})
	t.Run("does not forward the client's credentials or the ticket", func(t *testing.T) {
		*seen = nil
		get(base+"?ticket="+url.QueryEscape(mint(labs, "abc123", time.Minute)),
			map[string]string{"Authorization": "Bearer attacker", "Cookie": "session=1", "X-Migration-Token": "tok"})
		if len(*seen) != 1 {
			t.Fatalf("upstream saw %d requests", len(*seen))
		}
		h := (*seen)[0]
		if h.Get("Authorization") != "Bearer "+token || h.Get("Cookie") != "" || h.Get("X-Migration-Token") != "" {
			t.Errorf("upstream headers = %v", h)
		}
	})
	t.Run("refuses bad tickets without touching the pod", func(t *testing.T) {
		*seen = nil
		for name, path := range map[string]string{
			"none":         base,
			"garbage":      base + "?ticket=x.y",
			"expired":      base + "?ticket=" + url.QueryEscape(mint(labs, "abc123", -time.Second)),
			"other export": base + "?ticket=" + url.QueryEscape(mint(labs, "def456", time.Minute)),
			"other ns":     base + "?ticket=" + url.QueryEscape(mint("default", "abc123", time.Minute)),
		} {
			if resp, _ := get(path, nil); resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s: status %d, want 401", name, resp.StatusCode)
			}
		}
		if len(*seen) != 0 {
			t.Errorf("the pod was contacted %d times for refused tickets", len(*seen))
		}
	})
	t.Run("an unreachable pod is a 502, not a hang", func(t *testing.T) {
		dead := httptest.NewServer(http.NotFoundHandler())
		dead.Close()
		px2 := proxyServer(t, dead)
		resp, err := http.Get(px2.URL + base + "?ticket=" + url.QueryEscape(mint(labs, "abc123", time.Minute)))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadGateway {
			t.Errorf("status %d, want 502", resp.StatusCode)
		}
	})
}
