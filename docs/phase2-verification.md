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

## Not verified
- The real menu-entry path in a browser (the SPA minting a token through the
  proxy, then calling the API).
- Per-user RBAC with a *non-admin* user (all tests used an admin token).
- Who may open the UI at all: RBAC on `services/proxy` for the UI Service.

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
