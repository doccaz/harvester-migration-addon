// ova_credentials_test.go
//
// An OvaSource does not have to carry credentials: one created with kubectl, or by
// Harvester's own UI, may name no secret at all. The handlers used to answer 500
// ("OvaSource missing credentials secret name") for such a source, so it could be
// neither opened for editing nor updated from this UI.
package vmic

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"
	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/testutil"
)

func ovaSourceNoCredentials(name string) *unstructured.Unstructured {
	return crd("migration.harvesterhci.io/v1beta1", "OvaSource", name, map[string]interface{}{
		"url": "http://files.example.com/vm.ova",
	})
}

func getOva(t *testing.T, c *kube.Clients) *unstructured.Unstructured {
	t.Helper()
	o, err := c.Dynamic.Resource(kube.OVASourceGVR).Namespace(ns).Get(context.TODO(), "a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestGetOvaSourceWithoutCredentials(t *testing.T) {
	c := withObjects(nil, ovaSourceNoCredentials("a"))
	rr := testutil.Do(GetOvaSource(c), http.MethodGet, "/x", nil, vars)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var got map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	spec := got["spec"].(map[string]interface{})
	if spec["url"] != "http://files.example.com/vm.ova" {
		t.Errorf("url = %v", spec["url"])
	}
	if _, has := spec["username"]; has {
		t.Error("there is no secret, so there must be no username")
	}
}

func TestUpdateOvaSourceWithoutCredentials(t *testing.T) {
	t.Run("changing the URL needs no secret", func(t *testing.T) {
		c := withObjects(nil, ovaSourceNoCredentials("a"))
		rr := testutil.Do(UpdateOvaSource(c), http.MethodPut, "/x", CreateOvaSourcePayload{URL: "http://files.example.com/new.ova", HttpTimeoutSeconds: 30}, vars)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d, want 200: %s", rr.Code, rr.Body.String())
		}
		o := getOva(t, c)
		if u, _, _ := unstructured.NestedString(o.Object, "spec", "url"); u != "http://files.example.com/new.ova" {
			t.Errorf("url = %q", u)
		}
		if _, found, _ := unstructured.NestedMap(o.Object, "spec", "credentials"); found {
			t.Error("no credentials were given, so none must be invented")
		}
		secrets, _ := c.Clientset.CoreV1().Secrets(ns).List(context.TODO(), metav1.ListOptions{})
		if len(secrets.Items) != 0 {
			t.Errorf("%d secrets were created for an update without credentials", len(secrets.Items))
		}
	})

	t.Run("giving credentials creates the secret and links it", func(t *testing.T) {
		c := withObjects(nil, ovaSourceNoCredentials("a"))
		rr := testutil.Do(UpdateOvaSource(c), http.MethodPut, "/x", CreateOvaSourcePayload{URL: "http://files.example.com/vm.ova", Username: "alice", Password: "s3cret"}, vars)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d, want 200: %s", rr.Code, rr.Body.String())
		}
		o := getOva(t, c)
		name, _, _ := unstructured.NestedString(o.Object, "spec", "credentials", "name")
		if name != "a-ova-credentials" {
			t.Fatalf("credentials name = %q, want a-ova-credentials", name)
		}
		if n, _, _ := unstructured.NestedString(o.Object, "spec", "credentials", "namespace"); n != ns {
			t.Errorf("credentials namespace = %q", n)
		}
		s, err := c.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("secret not created: %v", err)
		}
		if s.StringData["username"] != "alice" || s.StringData["password"] != "s3cret" {
			t.Errorf("secret = %+v", s.StringData)
		}
	})

	t.Run("a leftover secret of that name is updated, not a conflict", func(t *testing.T) {
		c := withObjects([]runtime.Object{secretObj("a-ova-credentials")}, ovaSourceNoCredentials("a"))
		rr := testutil.Do(UpdateOvaSource(c), http.MethodPut, "/x", CreateOvaSourcePayload{URL: "http://files.example.com/vm.ova", Username: "bob", Password: "pw"}, vars)
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d, want 200: %s", rr.Code, rr.Body.String())
		}
		s, _ := c.Clientset.CoreV1().Secrets(ns).Get(context.TODO(), "a-ova-credentials", metav1.GetOptions{})
		if s.StringData["username"] != "bob" {
			t.Errorf("secret = %+v", s.StringData)
		}
	})
}

func TestDeleteOvaSourceWithoutCredentials(t *testing.T) {
	c := withObjects(nil, ovaSourceNoCredentials("a"))
	rr := testutil.Do(DeleteOvaSource(c), http.MethodDelete, "/x", nil, vars)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status %d, want 204: %s", rr.Code, rr.Body.String())
	}
	if _, err := c.Dynamic.Resource(kube.OVASourceGVR).Namespace(ns).Get(context.TODO(), "a", metav1.GetOptions{}); err == nil {
		t.Error("the source must be gone")
	}
}
