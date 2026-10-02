// respond.go

// Package httpx holds the small HTTP response helpers shared by every handler.
package httpx

import (
	"encoding/json"
	"net/http"

	log "github.com/sirupsen/logrus"
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
