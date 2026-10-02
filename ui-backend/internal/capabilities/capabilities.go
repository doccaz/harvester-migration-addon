// capabilities.go
package capabilities

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/httpx"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NEW: Capability configuration to send to frontend
type Config struct {
	HarvesterVersion string `json:"harvesterVersion"`
	HasAdvancedPower bool   `json:"hasAdvancedPower"` // v1.6.0+
}

// NEW: Handler to check Harvester version and features
// Gather reads the Harvester server-version setting and derives
// feature flags. On error it returns an "unknown" config alongside the error,
// so callers can choose to surface defaults (the HTTP handler) or record the
// failure (the support bundle).
func Gather(ctx context.Context, clients *kube.Clients) (Config, error) {
	if clients == nil || clients.Dynamic == nil {
		return Config{HarvesterVersion: "unknown", HasAdvancedPower: false}, fmt.Errorf("kubernetes client unavailable")
	}
	setting, err := clients.Dynamic.Resource(kube.SettingsGVR).Get(ctx, "server-version", metav1.GetOptions{})
	if err != nil {
		return Config{HarvesterVersion: "unknown", HasAdvancedPower: false}, err
	}

	version, _ := kube.NestedStringOrWarn(setting.Object, "value")

	// v1.6.0+ unlocks advanced power ops, disk bus type, and preflight checks.
	hasAdvanced := strings.Contains(version, "v1.6") ||
		strings.Contains(version, "v1.7") ||
		strings.Contains(version, "v1.8") ||
		strings.Contains(version, "v1.9") ||
		strings.Contains(version, "master")

	return Config{HarvesterVersion: version, HasAdvancedPower: hasAdvanced}, nil
}

func Handler(clients *kube.Clients) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caps, err := Gather(r.Context(), clients)
		if err != nil {
			// Permissions or a very old cluster — fall back to defaults.
			log.Warnf("Could not determine Harvester version: %v", err)
		}
		httpx.RespondWithJSON(w, http.StatusOK, caps)
	}
}
