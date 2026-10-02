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
