// recover_test.go
package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A handler that panics (a nil client in mock mode, a decoding bug) must become a JSON
// 500 for that request instead of a dropped connection, and must not stop the server.
func TestRecoverTurnsAPanicIntoAJSON500(t *testing.T) {
	h := Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/x", nil))
	if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "internal server error") {
		t.Errorf("status %d body %q", rr.Code, rr.Body)
	}
	if strings.Contains(rr.Body.String(), "boom") {
		t.Error("the panic value must not be echoed to the client")
	}
	// The next request is served normally.
	ok := Recover(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	rr = httptest.NewRecorder()
	ok.ServeHTTP(rr, httptest.NewRequest("GET", "/x", nil))
	if rr.Code != http.StatusTeapot {
		t.Errorf("a healthy handler was affected: %d", rr.Code)
	}
}

// A reverse proxy aborts its handler on purpose when the client goes away mid-download
// (panic(http.ErrAbortHandler)); net/http expects that panic to reach it, and then closes
// the connection quietly. Turning it into a logged error and a 500 is wrong.
func TestRecoverLetsAnAbortedHandlerPropagate(t *testing.T) {
	h := Recover(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("part of a download"))
		panic(http.ErrAbortHandler)
	}))
	rr := httptest.NewRecorder()
	var got interface{}
	func() {
		defer func() { got = recover() }()
		h.ServeHTTP(rr, httptest.NewRequest("GET", "/x", nil))
	}()
	if got != http.ErrAbortHandler {
		t.Fatalf("the abort must propagate to net/http, recovered %v", got)
	}
	if strings.Contains(rr.Body.String(), "internal server error") {
		t.Errorf("no error body may be written after an abort: %q", rr.Body)
	}
}

// Once a response has started there is no 500 left to send: appending one corrupts the
// stream and logs "superfluous WriteHeader".
func TestRecoverDoesNotAppendAnErrorToAResponseThatHasStarted(t *testing.T) {
	h := Recover(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("streamed"))
		panic("boom after the first byte")
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/x", nil))
	if rr.Code != http.StatusOK || rr.Body.String() != "streamed" {
		t.Errorf("status %d body %q, want the stream left as it was", rr.Code, rr.Body)
	}
}

// Downloads are streamed with Flush; the wrapper must not hide it.
func TestRecoverKeepsStreamingPossible(t *testing.T) {
	h := Recover(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f, ok := w.(http.Flusher)
		if !ok {
			t.Error("the response writer lost http.Flusher")
			return
		}
		_, _ = w.Write([]byte("chunk"))
		f.Flush()
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/x", nil))
	if !rr.Flushed {
		t.Error("Flush did not reach the underlying writer")
	}
	if http.NewResponseController(wrapperProbe(t)).Flush() != nil {
		t.Error("http.ResponseController cannot flush through the wrapper")
	}
}

// wrapperProbe returns the response writer a handler would see behind Recover.
func wrapperProbe(t *testing.T) http.ResponseWriter {
	t.Helper()
	var seen http.ResponseWriter
	Recover(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { seen = w })).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
	return seen
}

// Most handlers never call WriteHeader: the first Write starts the response (an
// implicit 200), and that counts as started too.
func TestRecoverCountsAnImplicitStatusAsStarted(t *testing.T) {
	h := Recover(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("first bytes"))
		panic("boom")
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/x", nil))
	if rr.Body.String() != "first bytes" {
		t.Errorf("body %q", rr.Body)
	}
}
