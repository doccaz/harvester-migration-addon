// respond_test.go
package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
