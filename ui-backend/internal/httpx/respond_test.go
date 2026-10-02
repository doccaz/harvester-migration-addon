// respond_test.go
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestRespondWithJSON(t *testing.T) {
	t.Run("normal payload", func(t *testing.T) {
		rr := httptest.NewRecorder()
		RespondWithJSON(rr, http.StatusOK, map[string]string{"key": "value"})

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
		if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		var result map[string]string
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if result["key"] != "value" {
			t.Errorf("expected value 'value', got '%s'", result["key"])
		}
	})

	t.Run("unmarshalable input returns 500", func(t *testing.T) {
		rr := httptest.NewRecorder()
		// A channel cannot be marshaled to JSON
		RespondWithJSON(rr, http.StatusOK, make(chan int))

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("expected status 500 for unmarshalable input, got %d", rr.Code)
		}
	})
}

func TestRespondWithError(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondWithError(rr, http.StatusBadRequest, "test error")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}

	var result map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result["error"] != "test error" {
		t.Errorf("expected error 'test error', got '%s'", result["error"])
	}
}

func TestRespondWithAPIError(t *testing.T) {
	gr := schema.GroupResource{Resource: "things"}
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", apierrors.NewNotFound(gr, "x"), http.StatusNotFound},
		{"already exists", apierrors.NewAlreadyExists(gr, "x"), http.StatusConflict},
		{"forbidden", apierrors.NewForbidden(gr, "x", errors.New("no")), http.StatusForbidden},
		{"unauthorized", apierrors.NewUnauthorized("expired"), http.StatusUnauthorized},
		{"invalid", apierrors.NewBadRequest("bad"), http.StatusBadRequest},
		{"wrapped API error keeps its status", fmt.Errorf("getting x: %w", apierrors.NewNotFound(gr, "x")), http.StatusNotFound},
		{"not an API error", errors.New("connection refused"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			RespondWithAPIError(rr, tc.err)
			if rr.Code != tc.want {
				t.Errorf("status %d, want %d", rr.Code, tc.want)
			}
			var body map[string]string
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil || body["error"] != tc.err.Error() {
				t.Errorf("body %q, want the error text", rr.Body.String())
			}
		})
	}
}
