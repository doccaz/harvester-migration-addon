// auth_test.go
package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTokenModeRejectsMissingToken(t *testing.T) {
	t.Setenv("USER_AUTH", "token")
	t.Setenv("KUBE_API_URL", "https://rancher.invalid/k8s/clusters/local")
	p, err := NewK8sProvider()
	if err != nil {
		t.Fatal(err)
	}
	called := false
	h := userScoped(p, func(*K8sClients) http.HandlerFunc {
		return func(http.ResponseWriter, *http.Request) { called = true }
	})
	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest("GET", "/api/v1/x", nil))
	if rr.Code != http.StatusUnauthorized || called {
		t.Fatalf("got %d (handler called=%v), want 401 and no call", rr.Code, called)
	}
}

func TestTokenModeUsesCallersTokenAndKeepsPathPrefix(t *testing.T) {
	var mu sync.Mutex
	var gotAuth, gotPath string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"NamespaceList","apiVersion":"v1","items":[]}`))
	}))
	defer api.Close()

	t.Setenv("USER_AUTH", "token")
	t.Setenv("KUBE_API_URL", api.URL+"/k8s/clusters/local")
	p, err := NewK8sProvider()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/api/v1/x", nil)
	req.Header.Set(userTokenHeader, "token-abc:secret")
	req.Header.Set("Authorization", "Bearer must-be-ignored")
	clients, err := p.For(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clients.Clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotAuth != "Bearer token-abc:secret" {
		t.Errorf("API saw Authorization %q, want the caller's token", gotAuth)
	}
	if gotPath != "/k8s/clusters/local/api/v1/namespaces" {
		t.Errorf("API saw path %q, want the Rancher prefix preserved", gotPath)
	}
}

func TestTokensAreNotShared(t *testing.T) {
	t.Setenv("USER_AUTH", "token")
	t.Setenv("KUBE_API_URL", "https://rancher.invalid")
	p, _ := NewK8sProvider()
	a := httptest.NewRequest("GET", "/", nil)
	a.Header.Set(userTokenHeader, "a")
	b := httptest.NewRequest("GET", "/", nil)
	b.Header.Set(userTokenHeader, "b")
	ca, _ := p.For(a)
	cb, _ := p.For(b)
	if ca == cb || ca.Clientset == cb.Clientset {
		t.Fatal("clients for different users must be distinct")
	}
	if p.base.BearerToken != "" {
		t.Fatal("base config must never hold a user token")
	}
}

func TestServiceAccountModeSharesClients(t *testing.T) {
	shared := &K8sClients{}
	p := &K8sProvider{mode: authServiceAccount, shared: shared}
	got, err := p.For(httptest.NewRequest("GET", "/", nil))
	if err != nil || got != shared {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestUnknownAuthModeFails(t *testing.T) {
	t.Setenv("USER_AUTH", "bogus")
	if _, err := NewK8sProvider(); err == nil {
		t.Fatal("expected error")
	}
}

func TestTokenModeRequiresAPIURL(t *testing.T) {
	t.Setenv("USER_AUTH", "token")
	t.Setenv("KUBE_API_URL", "")
	if _, err := NewK8sProvider(); err == nil {
		t.Fatal("expected error")
	}
}
