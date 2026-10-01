// auth.go
package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Auth modes, selected with USER_AUTH.
const (
	// authServiceAccount runs every request with the pod's own credentials
	// (development, single-user labs).
	authServiceAccount = "serviceaccount"
	// authToken runs every request with the caller's own token, so Kubernetes
	// RBAC applies per user. The pod's ServiceAccount is not used for API calls.
	authToken = "token"

	// userTokenHeader carries the caller's token. It cannot be Authorization:
	// the Kubernetes API server and Rancher authenticate that header themselves
	// and strip it before proxying, so the pod would never see it.
	userTokenHeader = "X-Migration-Token"
)

// K8sProvider hands out Kubernetes clients for a request.
type K8sProvider struct {
	mode string
	// shared is the ServiceAccount (or mock) client; nil in token mode.
	shared *K8sClients
	// base is the connection config without credentials, used in token mode.
	base *rest.Config
}

// NewK8sProvider builds the provider selected by USER_AUTH.
func NewK8sProvider() (*K8sProvider, error) {
	mode := strings.ToLower(os.Getenv("USER_AUTH"))
	if mode == "" {
		mode = authServiceAccount
	}
	switch mode {
	case authServiceAccount:
		clients, err := NewK8sClients()
		return &K8sProvider{mode: mode, shared: clients}, err
	case authToken:
		base, err := tokenBaseConfig()
		if err != nil {
			return nil, err
		}
		log.Infof("User token auth enabled; Kubernetes API at %s", base.Host)
		return &K8sProvider{mode: mode, base: base}, nil
	default:
		return nil, fmt.Errorf("unknown USER_AUTH %q (want %q or %q)", mode, authServiceAccount, authToken)
	}
}

// tokenBaseConfig describes where user tokens are presented. Rancher tokens are
// only valid at Rancher, not at kube-apiserver, so KUBE_API_URL normally points
// at https://rancher.cattle-system.svc/k8s/clusters/local.
func tokenBaseConfig() (*rest.Config, error) {
	host := os.Getenv("KUBE_API_URL")
	if host == "" {
		return nil, fmt.Errorf("USER_AUTH=token requires KUBE_API_URL")
	}
	cfg := &rest.Config{Host: host}
	if ca := os.Getenv("KUBE_API_CA_FILE"); ca != "" {
		cfg.TLSClientConfig.CAFile = ca
	}
	if os.Getenv("INSECURE_SKIP_TLS_VERIFY") == "true" {
		log.Warn("INSECURE_SKIP_TLS_VERIFY=true: API certificate is NOT verified")
		cfg.TLSClientConfig.Insecure = true
		cfg.TLSClientConfig.CAFile = ""
	}
	return cfg, nil
}

// For returns the clients to use for r. In token mode it fails when the request
// carries no token. The token is never logged.
func (p *K8sProvider) For(r *http.Request) (*K8sClients, error) {
	if p.mode != authToken {
		return p.shared, nil
	}
	token := strings.TrimSpace(r.Header.Get(userTokenHeader))
	if token == "" {
		return nil, errNoToken
	}
	cfg := rest.CopyConfig(p.base)
	cfg.BearerToken = token
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &K8sClients{Clientset: clientset, Dynamic: dyn}, nil
}

var errNoToken = fmt.Errorf("missing %s header", userTokenHeader)

// userScoped adapts a handler constructor to per-request clients, so existing
// handlers keep their `func(*K8sClients) http.HandlerFunc` shape.
func userScoped(p *K8sProvider, build func(*K8sClients) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clients, err := p.For(r)
		if err != nil {
			if err == errNoToken {
				respondWithError(w, http.StatusUnauthorized, err.Error())
				return
			}
			log.Errorf("Failed to build Kubernetes clients: %v", err)
			respondWithError(w, http.StatusInternalServerError, "Failed to build Kubernetes clients")
			return
		}
		build(clients)(w, r)
	}
}
