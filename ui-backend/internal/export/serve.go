// serve.go
//
// Serving one finished OVA out of an export volume.
//
// The API pod mounts only its own namespace's export volume, so for an export
// in any other namespace it cannot read the OVA. A short-lived pod in the
// export's namespace mounts that volume read-only and runs
// `vm-import-ui export-serve` — the same binary, like export-worker and
// export-cleanup. It serves exactly one file and nothing else, and only to a
// caller presenting its bearer token; the API pod holds that token and proxies
// browser downloads to it (see docs/export-download-design.md).
package export

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	// ServePath is the only path the serve pod answers.
	ServePath = "/ova"
	// ServePort is where the serve pod listens.
	ServePort = 8081

	defaultServeIdle = 30 * time.Minute
)

// idleTracker records request activity so the serve pod can exit when nobody
// has downloaded anything for a while. A download in flight is never idle, however long
// it takes.
type idleTracker struct {
	mu       sync.Mutex
	last     time.Time
	inflight int
	now      func() time.Time
}

func newIdleTracker(now func() time.Time) *idleTracker {
	return &idleTracker{last: now(), now: now}
}

func (t *idleTracker) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.mu.Lock()
		t.inflight++
		t.mu.Unlock()
		defer func() {
			t.mu.Lock()
			t.inflight--
			t.last = t.now()
			t.mu.Unlock()
		}()
		next.ServeHTTP(w, r)
	})
}

// expired reports whether nothing is in flight and the last request ended at
// least idle ago.
func (t *idleTracker) expired(idle time.Duration) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.inflight == 0 && t.now().Sub(t.last) >= idle
}

// serveHandler serves root/<target>.ova at ServePath to the holder of token.
// The file is resolved on every request, strictly inside root, so a name that
// escapes root fails closed. Range and conditional requests come from
// http.ServeContent, which streams without buffering.
func serveHandler(root, target, token string) (http.Handler, error) {
	if token == "" {
		return nil, fmt.Errorf("refusing to serve without a token")
	}
	if target == "" {
		return nil, fmt.Errorf("no target file given")
	}
	if _, err := safeExportPath(root, target+".ova"); err != nil {
		return nil, err
	}
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ServePath {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		path, err := safeExportPath(root, target+".ova")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f, err := os.Open(path)
		if err != nil {
			http.Error(w, "OVA not found", http.StatusNotFound)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || st.IsDir() {
			http.Error(w, "OVA not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/x-tar")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", target+".ova"))
		http.ServeContent(w, r, target+".ova", st.ModTime(), f)
	}), nil
}

// RunServe is the `export-serve` mode of the binary, run inside the serve pod.
// It returns the process exit code: 0 after the idle timeout, 1 on a failure.
func RunServe() int {
	root := os.Getenv("EXPORT_ROOT")
	target := os.Getenv("EXPORT_TARGET")
	token := os.Getenv("EXPORT_SERVE_TOKEN")
	addr := os.Getenv("EXPORT_SERVE_ADDR")
	if addr == "" {
		addr = ":" + strconv.Itoa(ServePort)
	}
	idle := defaultServeIdle
	if v := os.Getenv("EXPORT_SERVE_IDLE_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			idle = time.Duration(n) * time.Second
		}
	}

	h, err := serveHandler(root, strings.TrimSuffix(target, ".ova"), token)
	if err != nil {
		log.Errorf("Export serve cannot start: %v", err)
		return 1
	}
	tracker := newIdleTracker(time.Now)
	srv := &http.Server{
		Addr:              addr,
		Handler:           tracker.wrap(h),
		ReadHeaderTimeout: 10 * time.Second,
		// No write timeout: a multi-gigabyte download legitimately takes long.
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Infof("Serving %s.ova from %s on %s (idle exit after %s)", target, root, addr, idle)

	tick := time.NewTicker(idleCheckInterval(idle))
	defer tick.Stop()
	for {
		select {
		case err := <-errc:
			log.Errorf("Export serve failed: %v", err)
			return 1
		case <-tick.C:
			if tracker.expired(idle) {
				log.Infof("Idle for %s; shutting down", idle)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_ = srv.Shutdown(ctx)
				return 0
			}
		}
	}
}

// idleCheckInterval polls often enough to exit close to the deadline without
// spinning on a long timeout.
func idleCheckInterval(idle time.Duration) time.Duration {
	d := idle / 10
	if d < 200*time.Millisecond {
		d = 200 * time.Millisecond
	}
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}
