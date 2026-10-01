# Per-user identity for the UI (Phase 2)

## Problem
Every backend handler uses one shared ServiceAccount client. Anyone who can reach
the UI acts with that ServiceAccount's rights (secrets, migrations, Jobs). RBAC
narrowing (docs/rbac-audit.md) reduces the blast radius but cannot fix this.

## What we do NOT know yet
Which identity information reaches the pod when a user opens the UI from the
Harvester menu (NavLink `toService` -> Rancher -> kube-apiserver service proxy
-> pod). My expectation, unverified: kube-apiserver strips `Authorization` and
`Impersonate-*` after authenticating, so the pod sees only `X-Forwarded-*`.
The design below depends on the answer, so measure it first.

## Experiment (read-only apart from one throwaway namespace)
    kubectl apply -f hack/identity-probe.yaml
    kubectl wait -n identity-probe --for=condition=Ready pod/probe --timeout=90s
    # 1. directly through the API server (as the kubeconfig user)
    kubectl get --raw /api/v1/namespaces/identity-probe/services/http:probe:8080/proxy/cgi-bin/echo
    # 2. through Rancher/Harvester, as a UI user (token from Preferences > API Keys)
    curl -sk -H "Authorization: Bearer $TOKEN" \
      https://<harvester-vip>/k8s/clusters/local/api/v1/namespaces/identity-probe/services/http:probe:8080/proxy/cgi-bin/echo
    # 3. same URL with the browser session cookie, to see what a NavLink sends
    kubectl delete ns identity-probe

Record the headers seen in each case below.

## Options by outcome
| Pod receives | Design |
|---|---|
| A user bearer token (`Authorization`) | Backend builds a per-request client with that token. Server-side stays least-privilege; ServiceAccount only needed for non-user work. |
| `Impersonate-User`/`-Group` or other trusted user headers | Backend uses `Impersonate-*` with its SA (grant `impersonate` on users/groups, restricted by `resourceNames` where possible). Trust only if the pod is unreachable except via the proxy (NetworkPolicy). |
| Nothing identifying (my expectation) | (a) Browser-side: the SPA calls the Kubernetes API itself through Rancher's same-origin proxy for CR/Secret CRUD, so RBAC applies to the user; the backend keeps only server-side work (vCenter calls, bundles) and re-checks the caller with SelfSubjectAccessReview using a token the SPA forwards. Or (b) ship the UI as a Rancher UI extension (Phase 4 option), which gets user auth for free. |

Whichever applies, backend handlers should obtain their clients from one function
(`clientsFor(r *http.Request)`), replacing the global `k8sClients`, so the change
is a single seam. Until then the add-on must be described as "trusted network,
admin users only".

## Result
_Not measured yet._
