// serve_test.go
package export

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const serveTestToken = "s3cret-token"

// serveFixture writes target.ova under a fresh root and returns a test server
// in front of serveHandler, plus the file's bytes.
func serveFixture(t *testing.T) (*httptest.Server, string, []byte) {
	t.Helper()
	root := t.TempDir()
	data := make([]byte, 100000)
	for i := range data {
		data[i] = byte(i % 251)
	}
	if err := os.WriteFile(filepath.Join(root, "vm1.ova"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	h, err := serveHandler(root, "vm1", serveTestToken)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, root, data
}

func get(t *testing.T, method, url, auth string, hdr map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestServeRequiresTheBearerToken(t *testing.T) {
	srv, _, _ := serveFixture(t)
	for name, auth := range map[string]string{
		"none":         "",
		"wrong":        "Bearer nope",
		"no scheme":    serveTestToken,
		"prefix only":  "Bearer ",
		"token prefix": "Bearer " + serveTestToken[:4],
		"token plus":   "Bearer " + serveTestToken + "x",
	} {
		if resp := get(t, "GET", srv.URL+ServePath, auth, nil); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", name, resp.StatusCode)
		}
	}
	if resp := get(t, "GET", srv.URL+ServePath, "Bearer "+serveTestToken, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("valid token: status %d, want 200", resp.StatusCode)
	}
}

func TestServeAnswersOnlyTheOneFile(t *testing.T) {
	srv, root, _ := serveFixture(t)
	if err := os.WriteFile(filepath.Join(root, "other.ova"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	auth := "Bearer " + serveTestToken
	for _, p := range []string{"/", "/other.ova", "/vm1.ova", "/ova/", "/../etc/passwd", "/ova/../other.ova", "/.vm-import-ui/"} {
		if resp := get(t, "GET", srv.URL+p, auth, nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: status %d, want 404", p, resp.StatusCode)
		}
	}
	if resp := get(t, "POST", srv.URL+ServePath, auth, nil); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST: status %d, want 405", resp.StatusCode)
	}
}

func TestServeStreamsTheFileWithRangeAndHead(t *testing.T) {
	srv, _, data := serveFixture(t)
	auth := "Bearer " + serveTestToken

	resp := get(t, "GET", srv.URL+ServePath, auth, nil)
	body, _ := io.ReadAll(resp.Body)
	if string(body) != string(data) {
		t.Fatalf("full body differs (%d vs %d bytes)", len(body), len(data))
	}
	if resp.Header.Get("Accept-Ranges") != "bytes" {
		t.Error("Accept-Ranges must be bytes so browsers can resume")
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != `attachment; filename="vm1.ova"` {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if resp.Header.Get("Last-Modified") == "" {
		t.Error("Last-Modified is needed for If-Range resume")
	}

	resp = get(t, "GET", srv.URL+ServePath, auth, map[string]string{"Range": "bytes=1000-1999"})
	body, _ = io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusPartialContent || string(body) != string(data[1000:2000]) {
		t.Errorf("range: status %d, %d bytes, content match %v", resp.StatusCode, len(body), string(body) == string(data[1000:2000]))
	}
	if cr := resp.Header.Get("Content-Range"); cr != fmt.Sprintf("bytes 1000-1999/%d", len(data)) {
		t.Errorf("Content-Range = %q", cr)
	}

	resp = get(t, "HEAD", srv.URL+ServePath, auth, nil)
	if resp.StatusCode != http.StatusOK || resp.ContentLength != int64(len(data)) {
		t.Errorf("HEAD: status %d, length %d, want 200 and %d", resp.StatusCode, resp.ContentLength, len(data))
	}
}

func TestServeMissingFileIs404NotAnError(t *testing.T) {
	srv, root, _ := serveFixture(t)
	if err := os.Remove(filepath.Join(root, "vm1.ova")); err != nil {
		t.Fatal(err)
	}
	if resp := get(t, "GET", srv.URL+ServePath, "Bearer "+serveTestToken, nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("status %d, want 404", resp.StatusCode)
	}
}

func TestServeHandlerRefusesUnsafeConfig(t *testing.T) {
	root := t.TempDir()
	for name, c := range map[string]struct{ target, token string }{
		"no token":  {"vm1", ""},
		"no target": {"", "t"},
		"escape":    {"../outside", "t"},
		"absolute":  {"/../../etc/passwd", "t"},
	} {
		if _, err := serveHandler(root, c.target, c.token); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestIdleTrackerExpiresOnlyWhenNothingIsInFlight(t *testing.T) {
	now := time.Unix(1000, 0)
	tr := newIdleTracker(func() time.Time { return now })
	idle := 10 * time.Minute

	if tr.expired(idle) {
		t.Fatal("expired at start")
	}
	now = now.Add(idle)
	if !tr.expired(idle) {
		t.Fatal("should expire after the idle period with no requests")
	}

	// A request in flight keeps the pod alive however long it runs.
	release := make(chan struct{})
	started := make(chan struct{})
	h := tr.wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
	}))
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/ova", nil))
		close(done)
	}()
	<-started
	now = now.Add(5 * idle)
	if tr.expired(idle) {
		t.Fatal("expired while a download was in flight")
	}
	close(release)
	<-done
	if tr.expired(idle) {
		t.Fatal("the idle period must restart when the request ends")
	}
	now = now.Add(idle)
	if !tr.expired(idle) {
		t.Fatal("should expire again after going idle")
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

func TestRunServeServesThenExitsWhenIdle(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "vm1.ova"), []byte("hello ova"), 0o600); err != nil {
		t.Fatal(err)
	}
	addr := freeAddr(t)
	t.Setenv("EXPORT_ROOT", root)
	t.Setenv("EXPORT_TARGET", "vm1")
	t.Setenv("EXPORT_SERVE_TOKEN", serveTestToken)
	t.Setenv("EXPORT_SERVE_ADDR", addr)
	t.Setenv("EXPORT_SERVE_IDLE_SECONDS", "1")

	code := make(chan int, 1)
	go func() { code <- RunServe() }()

	var body []byte
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest("GET", "http://"+addr+ServePath, nil)
		req.Header.Set("Authorization", "Bearer "+serveTestToken)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			body, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if string(body) != "hello ova" {
		t.Fatalf("served %q", body)
	}
	select {
	case c := <-code:
		if c != 0 {
			t.Fatalf("exit code %d, want 0", c)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("did not exit after the idle period")
	}
}

func TestRunServeFailsWithoutAToken(t *testing.T) {
	t.Setenv("EXPORT_ROOT", t.TempDir())
	t.Setenv("EXPORT_TARGET", "vm1")
	t.Setenv("EXPORT_SERVE_TOKEN", "")
	// A server that wrongly starts would never return, so bound the wait.
	code := make(chan int, 1)
	go func() { code <- RunServe() }()
	select {
	case c := <-code:
		if c != 1 {
			t.Fatalf("exit code %d, want 1", c)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunServe started without a token")
	}
}
