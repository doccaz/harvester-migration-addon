# Phase 2: what is verified and what you need to run

## Verified (2026-10-01)
- Backend + frontend unit tests (token auth, 401 on bad/expired token, per-user
  clients, token cache, frontend token client).
- Backend in token mode against the lab Rancher: no token 401, bad token 401,
  valid token 200, token never logged, user logged at debug.
- Chart renders in every mode; token mode renders only the public Rancher CA
  (verified: 1 certificate, 0 private keys, verifies `rancher.cattle-system.svc`);
  server-side dry run on the lab OK.
- Static check: the API server never writes to disk (all writes are in the export
  worker Job), so `readOnlyRootFilesystem` is safe for the UI pod.

## Verified on the lab with the published v0.1.0 (2026-10-01)
`helm install mig mig/harvester-migration --version 0.1.0 --set controller.enabled=false
--set ui.auth.mode=token` on the lab (Harvester 1.8.2), then a port-forward to the pod:
- Image built by the release workflow runs as non-root with a read-only root
  filesystem (the Dockerfile and image are therefore verified too).
- TLS to `rancher.cattle-system.svc` verifies against the CA the chart copied
  (no insecure flag set).
- UI page 200; no token 401; bad token 401; valid token 200 with live
  `VmwareSource` objects and `capabilities` (`harvesterVersion v1.8.2`).
- The token does not appear in the pod log.

## Verified in the browser (v0.1.2, 2026-10-01)
Normal and Incognito windows, admin user, through the menu entry: the SPA mints one
Rancher token, retries its API calls (HTTP 200 x5), and the lists fill.
Two pitfalls found on the way, both fixed:
- `POST /v3/tokens` from a browser needs `Accept: application/json`; without it
  Rancher answers HTTP 201 with its HTML "API browser" page (token embedded in the
  markup), which looks like success but is not parseable.
- A failing mint must not repeat: the page polls every 10 s and each attempt
  creates a real token. The client now backs off for 5 minutes and deletes tokens
  it cannot use.

## Not verified
- Per-user RBAC with a *non-admin* user (all tests used an admin token).
- Who may open the UI at all: the chart now creates a `services/proxy` Role
  (`ui.access`), but the resourceNames spelling the apiserver actually checks has
  not been confirmed with a real non-admin user (see below).

## Non-admin test plan (needs a real limited Rancher user)
1. In the Harvester UI create a local user `migtest` (Users & Authentication) with
   a read-only or project-limited role, not an administrator.
2. Upgrade with `--set 'ui.access.users={<the user's id, e.g. user-abc12>}'` so
   the Role is bound to that user (find the id under Users, or `kubectl get
   users.management.cattle.io`).
3. Log in as `migtest` in an Incognito window and open the menu entry.
   - Expect the page to load (proxy access granted) and the lists to be empty or
     restricted, not the admin's data. Creating a source in a namespace the user
     cannot write to must fail with 403, not succeed through the UI.
   - Without step 2 the menu entry should fail at the proxy (403): that is the gate.
4. Record the result here.

## Commands to run (workstation with podman/docker and the lab kubeconfig)
    cd harvester-migration-addon
    podman build -t ghcr.io/doccaz/harvester-migration-ui:0.1.0 .
    podman run --rm --read-only --tmpfs /tmp -u 1001 -e USE_MOCK_DATA=true \
      -p 8081:8080 ghcr.io/doccaz/harvester-migration-ui:0.1.0 &
    curl -s -o /dev/null -w '%{http_code}\n' localhost:8081/        # expect 200
    # make it pullable by the lab (registry of your choice), then, UI-only because the
    # built-in controller add-on is enabled on the lab:
    helm install mig charts/harvester-migration -n harvester-system \
      --set controller.enabled=false --set ui.auth.mode=token
    kubectl -n harvester-system get pods -l app.kubernetes.io/name=harvester-migration-ui
    kubectl -n harvester-system logs deploy/mig-harvester-migration-ui | head
    # expect: "User token auth enabled; Kubernetes API at https://rancher.cattle-system.svc/..."
    # then open the "VM Migration" menu entry as a UI user.
