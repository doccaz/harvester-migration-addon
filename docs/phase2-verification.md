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

The browser pass (menu entry, Forklift tab, support bundle from About) is manual.

### Export enabled: the running-VM refusal (2026-10-02)
Upgraded with `export.enabled=true`, `export.storage.create=true`,
`export.storage.storageClass=harvester-longhorn`, then re-ran `hack/lab-smoke.sh`: 49 of 49,
and the export check now exercises the real guard instead of reporting 503: creating an
export of the **running** VM `labs/downstream-01` is refused with **409**.
Confirmed on the cluster, independent of the script: no export or cleanup Job exists, no
export volume was created in the VM's namespace under this release's name (so the refusal
happens before any side effect), the VM is still running, and no smoke-test resource was
left behind. Only the chart's own volume `harvester-system/mig-harvester-migration-exports`
(RWX, Bound) exists for this release.

## Successful export, end to end (2026-10-02, v0.3.0)

Exported `labs/bastion-galins-server` (stopped VM, 80 GiB virtual disk) through the UI API
with `hack/lab-export.sh`: the create call returned 202, the Job completed in 639 s and left
`bastion-galins-server-c0e85f41797f.ova` (4,556,208,640 bytes) on the export volume.

- Verified in the cluster (`hack/verify-ova.sh` via the helper pod): 9/9, with the manifest
  SHA-256 digests of the `.ovf` and `disk-0.vmdk` matching the real files.
- Fetched to the workstation with `hack/fetch-ova.sh`; the local SHA-256
  (`942efe46495dac3501d396cad7d463db09a1dccfe3d8f75ddd7bbc77f0dc69d4`) equals the one computed
  on the volume.
- Verified locally with `hack/verify-ova.sh`: 11/11, including validation against the DSP8023
  schema (`xmllint`) and `qemu-img` opening the disk as VMDK (80 GiB virtual, 4.24 GiB on disk).

Transfer lesson: `kubectl cp` and 256 MiB `kubectl exec ... dd` ranges were cut short (every
failure at 96-99.9% of the range). `fetch-ova.sh` now adapts the range size; 128 MiB ranges ran
without a single retry. Not yet exercised: importing this OVA back through Forklift's OVA provider.

## Bundled controller on the lab (2026-10-02, v0.3.0)

Switched the lab from the built-in `vm-import-controller` add-on to the controller bundled in
the `mig` release (plain `helm upgrade`, `controller.enabled=true`, after disabling the
built-in add-on; the conflict guard needs that). The lab release is a Helm release, not an
Addon CR.

- The controller came up as `harvester-vm-import-controller`, the four CRDs stayed served and
  the existing `VirtualMachineImport` objects were picked up unchanged.
- **Found: version mismatch.** v0.3.0 pins the upstream subchart at 1.9.0, so it ran
  `rancher/harvester-vm-import-controller:v1.9.0` on a Harvester 1.8.2 cluster (the built-in
  add-on ran v1.8.2). Overriding `harvester-vm-import-controller.image.tag=v1.8.2` worked.
- Both versions log the same loop for five completed imports whose temporary
  `VirtualMachineImage` objects no longer exist ("image-xxxxx not found, requeuing"). It is
  therefore not caused by 1.9.0 or by this chart; it comes from those old objects and the
  upstream controller. Their `VirtualMachineImport` objects were left untouched.
- Fix: the chart now pins the controller subchart to the Harvester minor it targets (1.8.2);
  see PLAN.md for the version policy. Not yet exercised: a new import run end to end.

## v0.3.1 on the lab (2026-10-02)

Upgraded `mig` to 0.3.1 (`helm upgrade --version 0.3.1 --reuse-values`, controller enabled, built-in
`vm-import-controller` add-on disabled). `hack/lab-smoke.sh`: **48/48**, backend and support-bundle
`meta.json` report 0.3.1 (a first run showed 2 failures that were the script's hard-coded expected
version; it now reads addon/harvester-migration.yaml). The bundled controller runs
`rancher/harvester-vm-import-controller:v1.8.2` from the pinned 1.8.2 subchart.

## Manual browser pass (2026-10-02, v0.3.1)

Done by hand on the lab: every tab of the UI works, support bundle creation works, VM export
works from the Export page, and logging in and out (token mint and expiry) works. The menu shows
two entries under Utilities: "VM Migration" (this add-on) and "VM Import UI" (the original
standalone `vm-import-ui` in its own namespace, a NodePort install other people still use); the
old one is deliberately left running for now.

Not exercised: re-importing an exported OVA through Forklift's OVA provider (the earlier OVA was
deleted; it needs a new export), and a new import run end to end.

## OVA download through the UI backend (2026-10-03, v0.4.0)

Upgraded `mig` to 0.4.0 (smoke test 48/48, backend and bundle report 0.4.0; the chart created the
ticket-key Secret). Exported `techday/sles16` (16 GiB virtual disk, 285 s, 1,484,376,576-byte OVA)
and downloaded it with `hack/lab-download.sh`:

- Ticket request: the serve pod `vm-export-serve-<id>` started in `techday` in about 22 s (first
  call; 202 while starting), 0 s once it was up.
- **Through a `kubectl port-forward`**: about 50 MB/s up to 759 MB, but a deliberately interrupted
  download never resumed (connection resets, no progress on every retry). The serve pod and the
  backend logged no error, so this is a port-forward artifact, not the download path. The script
  now detects a stalled resume, restarts the port-forward and gives up after 8 stalls.
- **Through the API-server service proxy** (the path the dashboard uses): interrupted twice on
  purpose (at 710 MB and 1.42 GB), resumed each time, complete in 32 s (about 46 MB/s).
- The downloaded file's SHA-256 (`d1f7d8b3...ef8d37`) equals the one computed on the volume, and
  `hack/verify-ova.sh` passes 11/11 (container, manifest digests, DSP8023 schema, `qemu-img`
  opens the disk as VMDK, 16 GiB virtual).

**Browser (2026-10-03):** the Download button works end to end; the file downloaded to the end and
its SHA-256 matches the volume's. Deleting the export in the UI removed the serve pod
(`kubectl get pod vm-export-serve-<id>` -> not found).

Not yet exercised: a multi-GB (~4.5 GB) download.

## v0.5.0-rc1 on the lab (2026-10-04)

- **Upgrade hung on storage, not on the release.** `helm upgrade mig ... --version 0.5.0-rc1` completed, but
  the new UI pod sat in `ContainerCreating` for 30+ minutes: `MountVolume.MountDevice failed ... has invalid
  controller count 2`. The export volume uses `harvester-longhorn`, a *migratable* Longhorn class (a block
  volume for VM live migration, one node at a time, no `share-manager-pvc-...` pod). The rolling update put the
  new pod on another node than the old one and Longhorn started a migration (second engine). It worked for 0.4.0
  only because both pods landed on the same node. Unstuck with `scale --replicas=0`, wait for `detached`,
  `scale --replicas=1`. Fixed in the chart (de26ed1): with export on the Deployment is replaced, not rolled,
  and the install notes warn about migratable classes; details and an example RWX class in
  docs/export-storage.md. The same limit applies to concurrent exports in one namespace.
- `hack/lab-smoke.sh` (`EXPECT_VERSION=0.5.0-rc1`): **55/55**, including the new engine-state, availability-state and
  inventory-warning checks; backend and support bundle report 0.5.0-rc1.
- Found in the old pod's log: the download proxy's client disconnects were logged as panics (`abort Handler`) by
  the panic-recovery wrapper; fixed in 22ca8e3 (not in rc1).
- Browser pass: pending.

## v0.5.0-rc2 on the lab (2026-10-04)
- Upgrade `0.5.0-rc1 -> 0.5.0-rc2` finished by itself: with export enabled the Deployment now uses the
  Recreate strategy, so there was no overlapping pod and no volume hang (the rc1 hang is described above).
- `hack/lab-smoke.sh` (`EXPECT_VERSION=0.5.0-rc2`): **55/55**.
- Browser pass, by hand: the plans footer's "Updated HH:MM:SS" moves every poll; "View in Harvester" works for
  vCenter sources, OVA sources, plans and (YAML view) Forklift providers and plans; the Forklift setup checklist,
  the export page and Download, and an OVA source created without `credentials` (details show "none") all work.
- The export volume still uses `harvester-longhorn` (migratable); the install notes warn about it. Moving to a
  real RWX class (docs/export-storage.md) is a follow-up, not a blocker.


## v0.5.1 (dependency bumps only), 2026-10-06

- Same code as v0.5.0 on Go 1.26, k8s.io/{api,apimachinery,client-go} 0.37.1, govmomi 0.56.0, gorilla/mux 1.8.1,
  logrus 1.10.2, sigs.k8s.io/yaml 1.6.0. CI green on `main`.
- Upgrade `0.5.0-rc2 -> 0.5.1-rc1` on the lab (Harvester 1.8.2) by repo URL (`helm upgrade ... --repo
  https://doccaz.github.io/harvester-migration-addon`); the pod came up on image `0.5.1-rc1`.
- `hack/lab-smoke.sh` (`EXPECT_VERSION=0.5.1-rc1`): **55/55**.
- Not repeated: the browser pass (no frontend change; the frontend only had testing-library dev dependency updates).
