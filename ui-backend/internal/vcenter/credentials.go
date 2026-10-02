// credentials.go
package vcenter

import (
	"context"
	"fmt"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/inventory"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Credentials struct {
	URL        string
	Username   string
	Password   string
	Datacenter string
}

// GatherInventory resolves a VmwareSource's endpoint and credentials and
// returns its inventory tree. Shared by the inventory endpoint and the support
// bundle so both go through one code path.
func GatherInventory(ctx context.Context, clients *kube.Clients, namespace, name string) (*inventory.Node, error) {
	sourceObj, err := clients.Dynamic.Resource(kube.VMwareSourceGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get VmwareSource: %w", err)
	}

	endpoint, found := kube.NestedStringOrWarn(sourceObj.Object, "spec", "endpoint")
	if !found {
		return nil, fmt.Errorf("VmwareSource missing spec.endpoint")
	}
	datacenter, _ := kube.NestedStringOrWarn(sourceObj.Object, "spec", "dc")

	secretName, found := kube.NestedStringOrWarn(sourceObj.Object, "spec", "credentials", "name")
	if !found {
		return nil, fmt.Errorf("VmwareSource missing credentials secret name")
	}
	secretNamespace, found := kube.NestedStringOrWarn(sourceObj.Object, "spec", "credentials", "namespace")
	if !found {
		return nil, fmt.Errorf("VmwareSource missing credentials secret namespace")
	}

	secret, err := clients.Clientset.CoreV1().Secrets(secretNamespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials secret: %w", err)
	}

	creds := Credentials{
		URL:        endpoint,
		Username:   string(secret.Data["username"]),
		Password:   string(secret.Data["password"]),
		Datacenter: datacenter,
	}

	return GetInventory(ctx, creds)
}
