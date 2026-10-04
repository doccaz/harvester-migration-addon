// Package engines reports whether each migration engine can be used, and if not,
// why. "Absent" and "I could not tell" are different answers: a Forklift add-on that
// is not installed needs installing, a user without permission needs a role, and a
// failed API call needs a retry. Treating all three as "not available" (as the
// Forklift check once did) sends people to fix the wrong thing.
package engines

import (
	"context"
	"os"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"
)

// State says how an engine stands.
type State string

const (
	StateAvailable    State = "available"
	StateNotInstalled State = "not-installed" // its CRDs are not served: the add-on is absent
	StateNotReady     State = "not-ready"     // installed, but what marks it ready is missing
	StateForbidden    State = "forbidden"     // the caller may not read it, so its state is unknown
	StateDisabled     State = "disabled"      // switched off in this deployment
	StateUnknown      State = "unknown"       // the check itself failed
)

// Status is one engine's answer.
type Status struct {
	Available bool   `json:"available"`
	State     State  `json:"state"`
	Message   string `json:"message,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

func available(ns string) Status {
	return Status{Available: true, State: StateAvailable, Namespace: ns}
}

// crdMissing reports whether err says the resource type itself does not exist, as
// opposed to one object of it not existing: the API server names the object in the
// status of the latter and not of the former.
func crdMissing(err error) bool {
	if meta.IsNoMatchError(err) {
		return true
	}
	if !apierrors.IsNotFound(err) {
		return false
	}
	if s, ok := err.(apierrors.APIStatus); ok {
		d := s.Status().Details
		return d == nil || d.Name == ""
	}
	return false
}

// classify turns the error of a read into a status. objectMissing is what an
// ordinary "not found" for the object we asked for means for this engine.
func classify(err error, what, ns string, objectMissing Status) Status {
	switch {
	case err == nil:
		return available(ns)
	case crdMissing(err):
		return Status{State: StateNotInstalled, Namespace: ns, Message: what + " is not installed on this cluster."}
	case apierrors.IsNotFound(err):
		return objectMissing
	case apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err):
		return Status{State: StateForbidden, Namespace: ns, Message: "You do not have permission to read " + what + ", so its state is unknown."}
	default:
		return Status{State: StateUnknown, Namespace: ns, Message: "Could not check " + what + ": " + err.Error()}
	}
}

// Forklift checks Forklift by its "host" Provider in namespace: the Forklift
// controller creates it once it is running, so its presence means the engine is
// usable, and its absence with the CRDs present means it is still coming up.
func Forklift(ctx context.Context, clients *kube.Clients, namespace string) Status {
	if namespace == "" {
		namespace = "forklift"
	}
	_, err := clients.Dynamic.Resource(kube.ForkliftProviderGVR).Namespace(namespace).Get(ctx, "host", metav1.GetOptions{})
	return classify(err, "Forklift", namespace, Status{
		State: StateNotReady, Namespace: namespace,
		Message: "Forklift host Provider not found in namespace " + namespace + ". Forklift features are unavailable.",
	})
}

// VMIC checks the VM Import Controller by whether its VirtualMachineImport CRD is served.
func VMIC(ctx context.Context, clients *kube.Clients) Status {
	_, err := clients.Dynamic.Resource(kube.VMIGVR).Namespace("").List(ctx, metav1.ListOptions{Limit: 1})
	return classify(err, "the VM Import Controller", "", Status{State: StateNotReady, Message: "The VM Import Controller is not ready."})
}

// Export reports whether VM export is switched on in this deployment (the chart sets
// the export environment only when export.enabled is true).
func Export() Status {
	if os.Getenv("EXPORT_PVC") == "" || os.Getenv("EXPORT_IMAGE") == "" {
		return Status{State: StateDisabled, Message: "VM export is not enabled; set export.enabled=true in the chart values."}
	}
	return Status{Available: true, State: StateAvailable}
}

// All reports every engine, keyed by the id the UI registry uses.
func All(ctx context.Context, clients *kube.Clients) map[string]Status {
	return map[string]Status{
		"vmic":     VMIC(ctx, clients),
		"forklift": Forklift(ctx, clients, ""),
		"export":   Export(),
	}
}
