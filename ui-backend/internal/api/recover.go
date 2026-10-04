// recover.go
package api

import (
	"net/http"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/httpx"
	log "github.com/sirupsen/logrus"
)

// startedWriter remembers whether the response has begun, and keeps the streaming
// interfaces reachable: downloads are flushed as they arrive, and net/http's
// ResponseController finds the real writer through Unwrap.
type startedWriter struct {
	http.ResponseWriter
	started bool
}

func (w *startedWriter) WriteHeader(code int) {
	w.started = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *startedWriter) Write(b []byte) (int, error) {
	w.started = true
	return w.ResponseWriter.Write(b)
}

func (w *startedWriter) Flush() {
	w.started = true
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *startedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Recover turns a panic in any handler into a 500 response instead of
// crashing the connection (which the client sees as a hung/empty reply). It keeps
// the server responsive when a dependency is unavailable — e.g. running with
// USE_MOCK_DATA and no cluster, where the Kubernetes clients are nil.
//
// Two panics are not errors to report. http.ErrAbortHandler is how a handler (the
// download proxy, when the client disconnects mid-stream) asks net/http to drop the
// connection quietly, so it is passed on. And once the response has started there is
// no 500 left to send: appending one would corrupt the stream.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &startedWriter{ResponseWriter: w}
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			log.Errorf("panic serving %s %s: %v", r.Method, r.URL.Path, rec)
			if !sw.started {
				httpx.RespondWithError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(sw, r)
	})
}
