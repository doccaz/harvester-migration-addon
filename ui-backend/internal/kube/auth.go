// auth.go
package kube

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/doccaz/harvester-migration-addon/ui-backend/internal/httpx"

	log "github.com/sirupsen/logrus"
	authv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// Provider hands out Kubernetes clients for a request.
type Provider struct {
	mode string
	// shared is the ServiceAccount (or mock) client; nil in token mode.
	shared *Clients
	// base is the connection config without credentials, used in token mode.
	base *rest.Config

	// validated caches tokens that recently passed validation, keyed by hash,
	// so the extra authentication round trip is not paid on every request.
	mu        sync.Mutex
	validated map[[32]byte]time.Time
}

// validationTTL bounds how long a successful token check is reused. An expired
// or revoked token is therefore rejected within this window.
const validationTTL = 30 * time.Second

// NewServiceAccountProvider returns a provider that always hands out the given
// shared clients (nil is allowed, for mock mode and tests).
func NewServiceAccountProvider(shared *Clients) *Provider {
	return &Provider{mode: authServiceAccount, shared: shared}
}

// NewProvider builds the provider selected by USER_AUTH.
func NewProvider() (*Provider, error) {
	mode := strings.ToLower(os.Getenv("USER_AUTH"))
	if mode == "" {
		mode = authServiceAccount
	}
	switch mode {
	case authServiceAccount:
		clients, err := NewClients()
		return &Provider{mode: mode, shared: clients}, err
	case authToken:
		base, err := tokenBaseConfig()
		if err != nil {
			return nil, err
		}
		log.Infof("User token auth enabled; Kubernetes API at %s", base.Host)
		return &Provider{mode: mode, base: base}, nil
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
func (p *Provider) For(r *http.Request) (*Clients, error) {
	if p.mode != authToken {
		return p.shared, nil
	}
	token := strings.TrimSpace(r.Header.Get(userTokenHeader))
	if token == "" {
		return nil, errNoToken
	}
	clients, err := p.clientsForToken(token)
	if err != nil {
		return nil, err
	}
	if err := p.validate(r.Context(), token, clients); err != nil {
		return nil, err
	}
	return clients, nil
}

// forUnvalidated builds clients from the request's token without checking it.
func (p *Provider) forUnvalidated(r *http.Request) (*Clients, error) {
	return p.clientsForToken(strings.TrimSpace(r.Header.Get(userTokenHeader)))
}

func (p *Provider) clientsForToken(token string) (*Clients, error) {
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
	return &Clients{Clientset: clientset, Dynamic: dyn}, nil
}

// validate asks the API who the token belongs to. It turns a bad or expired token
// into errUnauthorized (HTTP 401) up front, which is what lets the frontend mint a
// fresh one; without it handlers would surface the failure as an opaque 500.
func (p *Provider) validate(ctx context.Context, token string, clients *Clients) error {
	key := sha256.Sum256([]byte(token))
	p.mu.Lock()
	if until, ok := p.validated[key]; ok && time.Now().Before(until) {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	review, err := clients.Clientset.AuthenticationV1().SelfSubjectReviews().Create(ctx, &authv1.SelfSubjectReview{}, metav1.CreateOptions{})
	switch {
	case apierrors.IsUnauthorized(err):
		return errUnauthorized
	case err != nil:
		return fmt.Errorf("validating token: %w", err)
	}
	log.WithField("user", review.Status.UserInfo.Username).Debug("Authenticated request")

	p.mu.Lock()
	if p.validated == nil {
		p.validated = map[[32]byte]time.Time{}
	}
	now := time.Now()
	for k, until := range p.validated { // drop expired entries
		if now.After(until) {
			delete(p.validated, k)
		}
	}
	p.validated[key] = now.Add(validationTTL)
	p.mu.Unlock()
	return nil
}

var errUnauthorized = fmt.Errorf("token rejected")

var errNoToken = fmt.Errorf("missing %s header", userTokenHeader)

// Scoped adapts a handler constructor to per-request clients, so existing
// handlers keep their `func(*Clients) http.HandlerFunc` shape.
func Scoped(p *Provider, build func(*Clients) http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clients, err := p.For(r)
		if err != nil {
			if err == errNoToken || err == errUnauthorized {
				httpx.RespondWithError(w, http.StatusUnauthorized, err.Error())
				return
			}
			log.Errorf("Failed to build Kubernetes clients: %v", err)
			httpx.RespondWithError(w, http.StatusInternalServerError, "Failed to build Kubernetes clients")
			return
		}
		build(clients)(w, r)
	}
}
