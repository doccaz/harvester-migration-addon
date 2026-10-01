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

## Result (2026-10-01, lab, Harvester 1.8.x, kubeconfig = token user via the VIP)
Case 1 (service proxy via `https://<vip>`, i.e. ingress -> Rancher -> kube-apiserver):
the pod received **no** `Authorization`, **no** `Impersonate-*`, and no user/group
header. Only: `X-Forwarded-{For,Host,Port,Proto,Scheme,Uri}`, `X-Real-IP`,
`X-Request-Id`, `User-Agent`, `Host`, kubectl session headers.

Cases 2 and 3 (explicit `/k8s/clusters/local/...` URL with a UI API key; browser
cookie from a NavLink) were not run separately. They use the same kube-apiserver
service-proxy mechanism, so the expected result is the same.

**Conclusion: the pod cannot learn the caller's identity from the proxy.** A
shared ServiceAccount behind the proxy authorises "anyone who may open the
proxy URL" (RBAC `services/proxy` on the UI Service), nothing finer.

## Design candidates now that identity is not forwarded
1. **Token forwarding (interim).** The SPA obtains a user token and sends it to
   the backend in a header; the backend makes every Kubernetes call with that
   token (per-request client from `clientsFor(r)`), so RBAC applies per user and
   the ServiceAccount needs almost no rights. Open questions to measure:
   how the SPA gets a token (Rancher `/v3/tokens` with the session cookie + CSRF
   header, or user-pasted API key), and whether that token is accepted from
   inside the cluster at `https://rancher.cattle-system.svc/k8s/clusters/local`.
2. **Browser-side CRUD.** The SPA talks to the Kubernetes API itself through
   Rancher's same-origin proxy; the backend keeps only server-side work. Largest
   frontend change; backend still needs to authorise its own calls (see 1).
3. **Rancher UI extension** (Phase 4 option). Native user auth, no token
   plumbing; biggest rewrite.
4. **Gate only (floor).** Restrict who may open the proxy URL via RBAC on
   `services/proxy`, plus NetworkPolicy, and document "admin users only".
   Always do this regardless of 1-3.
