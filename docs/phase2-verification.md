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

## Access Role verified against the real API server (v0.2.0, 2026-10-02)
`hack/verify-access-rbac.sh` on the lab node, chart installed with
`--set 'ui.access.users={migtest}'`: 12 of 12 checks pass.
- `migtest` is allowed GET, POST and DELETE through the service proxy; `nobody` is
  denied all three (so the Role's `get/create/update/delete` verbs are what gates it).
- All four spellings of the service name (`svc`, `svc:8080`, `http:svc:8080`,
  `http:svc:http`) are accepted for the bound user.
- The Role does not open any other service in the namespace (`cdi-api` is denied).
- `migtest` cannot list/get Secrets or create sources, imports or Forklift
  providers: reaching the page grants no data access.
Caveat: the script treats anything other than "Forbidden" as allowed, so "allow"
means "authorisation passed", not "the UI returned 200" (the harmless POST/DELETE
are answered or rejected by the UI itself).

## Not verified
- A real login as a limited user. Harvester's embedded Rancher has no user
  management UI and no global roles (only users, tokens and auth settings exist),
  so this was covered at the Kubernetes RBAC level by impersonation instead.
  Resource permissions in token mode are enforced by the API server for the
  caller's token, which impersonation exercises equivalently; the token flow
  itself was verified with an admin in the browser.

## v0.3.0 on the lab (2026-10-02)
`hack/lab-smoke.sh` against the deployed v0.3.0 pod (Harvester 1.8.2, token mode): 49 of 49
checks pass.
- Stamped version `v0.3.0` in the pod log and in the support bundle's meta.json.
- 401 without or with a bad token; the page loads without a token.
- All 12 read routes 200; support bundle 200, 29 members, no secret values.
- Status mapping as designed: missing VMIC/Forklift plan YAML 404, creating an existing
  namespace 409, OVA inventory with an invalid namespace 400.
- Writes: Forklift OVA and vSphere providers created (201), stored `apiVersion` is
  `forklift.konveyor.io/v1beta1` and the `empty-vddk-init-image` annotation is set (the
  string literals a refactor script once corrupted), duplicate create 409, delete 204,
  secret removed, second delete 404; VMware source create/get/duplicate/delete likewise.
- The same script run read-only against v0.2.0 failed exactly the four behaviours this
  release changes, so it discriminates between versions.

Not covered: export is not configured on the lab pod (503), so the running-VM refusal
(409) is covered by unit tests only; the browser pass (menu entry, Forklift tab, support
bundle from About) is manual.
