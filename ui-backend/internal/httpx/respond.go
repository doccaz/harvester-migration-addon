// respond.go

// Package httpx holds the small HTTP response helpers shared by every handler.
package httpx

import (
	"encoding/json"
	"errors"
	"net/http"

	log "github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// RespondWithJSON writes payload as JSON with the given status code. A payload
// that cannot be marshalled yields a 500 with a JSON error body.
func RespondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("Failed to marshal JSON response: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if _, writeErr := w.Write([]byte(`{"error":"internal server error: failed to marshal response"}`)); writeErr != nil {
			log.Warnf("Failed to write error response: %v", writeErr)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if _, writeErr := w.Write(response); writeErr != nil {
		log.Warnf("Failed to write response body: %v", writeErr)
	}
}

// RespondWithError writes {"error": message} with the given status code.
func RespondWithError(w http.ResponseWriter, code int, message string) {
	RespondWithJSON(w, code, map[string]string{"error": message})
}

// RespondWithAPIError answers with the HTTP status the Kubernetes API server
// reported for err (404 not found, 409 already exists or conflict, 403 forbidden,
// 422 invalid, ...), and 500 when err did not come from the API server. The
// message is the error text, as with RespondWithError.
func RespondWithAPIError(w http.ResponseWriter, err error) {
	RespondWithAPIErrorMsg(w, err, err.Error())
}

// RespondWithAPIErrorMsg is RespondWithAPIError with a caller-supplied message
// (typically "Failed to create X: "+err.Error()), so existing wording is kept
// while the status follows the API server's.
func RespondWithAPIErrorMsg(w http.ResponseWriter, err error, message string) {
	RespondWithError(w, StatusFor(err), message)
}

// StatusFor returns the HTTP status the Kubernetes API server reported for err, or
// 500 when err did not come from the API server.
func StatusFor(err error) int {
	var status apierrors.APIStatus
	if errors.As(err, &status) {
		if c := int(status.Status().Code); c >= 400 && c < 600 {
			return c
		}
	}
	return http.StatusInternalServerError
}
