// testutil.go

// Package testutil holds helpers shared by the tests of several packages: fake
// Kubernetes clients and a tiny HTTP request runner. Only tests import it.
package testutil

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/kube"

	"github.com/gorilla/mux"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// NewClients creates kube.Clients backed by fake clientsets for testing.
func NewClients(objects ...runtime.Object) *kube.Clients {
	scheme := runtime.NewScheme()
	fakeClientset := fake.NewSimpleClientset(objects...)
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme)
	return &kube.Clients{
		Clientset: fakeClientset,
		Dynamic:   fakeDynamic,
	}
}

// NewClientsWithDynamic creates kube.Clients with pre-seeded dynamic objects.
func NewClientsWithDynamic(coreObjects []runtime.Object, dynamicObjects ...runtime.Object) *kube.Clients {
	scheme := runtime.NewScheme()
	fakeClientset := fake.NewSimpleClientset(coreObjects...)
	fakeDynamic := dynamicfake.NewSimpleDynamicClient(scheme, dynamicObjects...)
	return &kube.Clients{
		Clientset: fakeClientset,
		Dynamic:   fakeDynamic,
	}
}

// NewClientsWithListKinds returns clients whose fake dynamic client can LIST the
// given resources. The fake guesses a resource name from an object's kind
// ("NetworkAttachmentDefinition" -> "networkattachmentdefinitions"), which is wrong
// for hyphenated resources such as network-attachment-definitions; declare those
// here and create the objects through the client with the real GVR.
func NewClientsWithListKinds(listKinds map[schema.GroupVersionResource]string) *kube.Clients {
	return &kube.Clients{
		Clientset: fake.NewSimpleClientset(),
		Dynamic:   dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds),
	}
}

// Fail makes the fake clients answer every request for verb ("get", "list",
// "create", "update", "delete") on resource ("secrets", "virtualmachineimports",
// ...) with err, so a test can see how a handler maps API errors to HTTP statuses.
func Fail(clients *kube.Clients, verb, resource string, err error) {
	react := func(k8stesting.Action) (bool, runtime.Object, error) { return true, nil, err }
	clients.Clientset.(*fake.Clientset).PrependReactor(verb, resource, react)
	clients.Dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor(verb, resource, react)
}

// Do creates and executes a test HTTP request.
func Do(handler http.HandlerFunc, method, path string, body interface{}, vars map[string]string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != nil {
		bodyBytes, _ := json.Marshal(body)
		req = httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if vars != nil {
		req = mux.SetURLVars(req, vars)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// --- Tests ---
