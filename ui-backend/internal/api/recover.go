// recover.go
package api

import (
	"net/http"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/httpx"
	log "github.com/sirupsen/logrus"
)

// Recover turns a panic in any handler into a 500 response instead of
// crashing the connection (which the client sees as a hung/empty reply). It keeps
// the server responsive when a dependency is unavailable — e.g. running with
// USE_MOCK_DATA and no cluster, where the Kubernetes clients are nil.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Errorf("panic serving %s %s: %v", r.Method, r.URL.Path, rec)
				httpx.RespondWithError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
