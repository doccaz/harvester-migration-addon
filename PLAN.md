# Harvester Migration Add-on — analysis and phased plan

Status (2026-10-02): Phases 0–2 done and released (current release v0.2.0, https://github.com/doccaz/harvester-migration-addon); Phase 3 in progress; Phases 4–6 not started. Last updated alongside the Phase 3 `inventory` extraction.

## 1. What exists today

| | vm-import-controller | vm-import-ui | harvester-ui-extension (upstream) |
|---|---|---|---|
| Role | Reconciles `VirtualMachineImport` + `VmwareSource`/`OvaSource`/`OpenstackSource` CRs; converts disks with qemu; serves them on `:8080` for `VirtualMachineImage` | Web wizard + REST API over **two engines** (VMIC and Forklift), vCenter explorer, support bundle, VM export (Harvester → OVA) | Plain CRUD edit/list pages for the VMIC CRDs (`edit/migration.harvesterhci.io.*.vue`) |
| Language | Go 1.26, wrangler/lasso generated typed clients, k8s 0.35, govmomi 0.52 | Go 1.24, flat `package main`, gorilla/mux, **unstructured dynamic client**, k8s 0.28, govmomi 0.33; React 18 + Tailwind, `App.js` ~5.7k lines | Vue / Rancher shell |
| Packaging | Built-in Harvester add-on (`harvester-vm-import-controller` chart in harvester/charts, wired in harvester/addons `rancherd-22-addons.yaml`) | Own chart + NavLink, NodePort 32000, image on ghcr.io/doccaz | Ships inside Harvester UI |
| Style | logrus `WithFields`, dapper/scripts, strict golangci (gosec, prealloc, staticcheck), `revive.toml` | logrus aliased `log`, giant `handlers.go` (~2.2k lines), no linter config seen | Rancher conventions |

Forklift is already a separate experimental Harvester add-on, packaged by [harvester/forklift-packaging](https://github.com/harvester/forklift-packaging) (cloned to `reference/forklift-packaging`, Forklift v2.9.2). Its shape matters for the design:

- It is **not a plain chart**: the `forklift-operator` chart installs an *Ansible-based operator* (patched `watches.yml`/role for non-OpenShift) plus the Forklift CRDs, and the actual stack (controller, API, validation, inventory, virt-v2v, populators, OVA provider server) only appears after a **`ForkliftController` CR** is created. Your `forklift-controller.yaml` is that CR (`feature_ui_plugin: "false"`).
- **`cert-manager` is a prerequisite** (README: apply it before deploying).
- It builds ~12 SLE-BCI images (`registry.rancher.com/harvester/*`), several of which (virt-v2v, populators, ova-provider-server) are only used by specific migration paths.
- Chart is 0.1.0 with `tag: main-head`; the images are unreleased/dev-tagged in places, consistent with the experimental label.

The two `vm-import-ui-extension*` directories show a Rancher UI-extension route was already prototyped; `-wip` is not a git repo and uses an SDK that does not match `@rancher/shell`, so treat it as scrap.

## 2. Hard constraints found in the code

1. **Names are hard-coded in Harvester.** `pkg/util/constants.go:88` (`vm-import-controller-harvester-vm-import-controller`) and the setting validator (`validator.go:68`, `:1138`, label `app.kubernetes.io/name=harvester-vm-import-controller`) find the controller's volumes. Renaming the Deployment/labels breaks storage-network handling. The combined chart must preserve them (`fullnameOverride: harvester-vm-import-controller`) or we need a Harvester-side change.
2. **Port 8080 is the controller's disk file server.** The UI cannot share that port in one pod.
3. **Two controllers must never run together.** CRDs are created at runtime (`crd.Create`); no leader election is visible. Enabling the built-in `vm-import-controller` add-on *and* a new add-on that also runs the controller would double-reconcile every CR.
4. **Security.** The UI backend uses its own cluster-wide ServiceAccount (secrets create/delete, Jobs for export) behind an unauthenticated NodePort. Fine for a lab, not acceptable for a shipped add-on.
5. **Controller needs** 2–4 GiB RAM, runs as root, uses `/tmp` (optionally a PVC) for converted disks. The UI is tiny. Different resource profiles argue for separate Pods.
6. **Version skew.** k8s 0.28 / govmomi 0.33 (UI) vs 0.35 / 0.52 (controller). A single Go module forces the UI up to the controller's versions.

## 3. Options

**A. Merge the deployment, not the code (recommended).**
One chart, one `Addon` CR, two Deployments (controller from the unchanged upstream image, UI from its own image). Controller as a Helm subchart/dependency of the upstream chart.

**B. Merge into one binary / one repo of Go code.** Fork the controller, fold the UI backend into it (shared informers instead of dynamic client).

**C. UI-only: Rancher UI extension replacing the SPA.** Native Harvester look, uses the user's Rancher auth. Largest rewrite of the 5.7k-line frontend.

### Pros / cons

| | A. One chart, two Deployments | B. One binary | C. UI extension |
|---|---|---|---|
| Meets "enable one add-on" | Yes | Yes | Needs controller add-on too |
| Fork maintenance | None (consume upstream image) | **Permanent rebase on harvester/vm-import-controller** | None |
| Upstream-ability | Chart can be proposed to harvester/charts | Low; maintainers unlikely to take UI + Forklift + export | High for VMIC pages, but upstream already has them |
| Typed clients, informer cache, less API chatter | No | Yes | n/a |
| Dependency churn | None | UI forced onto k8s 0.35, govmomi 0.52, wrangler | Node/@rancher/shell churn |
| Blast radius of UI bug | UI Pod only | Controller crash-loops affect migrations | Browser only |
| Auth | Must solve (see Phase 2) | Same | Free (Rancher auth) |
| Effort | Low–medium | High | High |

**Recommendation:** A now; consider a *shared Go library* later rather than a merged binary. B's gain (typed clients) can be had by importing the controller's `pkg/apis` types into the UI backend without forking.

## 4. Your idea: one common UI module for both engines

Agreed, and most of it already exists in vm-import-ui, which is the only code that drives both engines. What is missing is a clean seam:

- Define an **engine interface** in the backend: `ListSources/Providers`, `Inventory`, `CreatePlan`, `RunPlan`, `Status`, `Logs`, `Delete`. Implement `vmic` and `forklift` engines behind it; handlers stop containing per-engine branching.
- Frontend: split `App.js` into feature modules (`engines/vmic`, `engines/forklift`, `export`, `shared`) with an engine registry, so an engine appears only if its CRD/add-on is detected (the support bundle already tolerates a missing Forklift CRD).
- **Do not fold Forklift's installation into our chart.** Because of the operator + `ForkliftController` CR + cert-manager sequence and the image count, bundling it would double the add-on's footprint and couple our release cadence to Forklift's. Keep `forklift-operator` a separate add-on owned by forklift-packaging; our add-on only *detects and guides*: UI shows a readiness checklist (Addon present? cert-manager present? `ForkliftController` Ready? Providers CRD served?) and can offer to create the `ForkliftController` CR from a template. (Harvester Addons cannot declare dependencies.)
- Chart values: `engines.vmic.enabled` / `engines.forklift.enabled` toggle UI modules and RBAC only. Forklift RBAC (providers, plans, networkmaps, storagemaps, migrations, pods/log in the forklift namespace) is requested only when that engine is on.
- Forklift-specific knobs the UI already handles (VDDK init image, OVA provider via NFS, skip-TLS) stay in the UI; note the OVA provider needs the `ova-provider-server` image from forklift-packaging.
- Version coupling: record the supported Forklift range (packaging is validated against v2.9.2) and have the capabilities endpoint report the detected version so the UI can disable options that CRDs don't have (e.g. new populators).
- Keep Harvester's own simple VMIC pages in place; ours is the "advanced wizard" and links to it, rather than replacing it.

## 4b. Relationship to the built-in Harvester UI

Harvester's own UI (`harvester-ui-extension`) already has plain CRUD pages for the VMIC CRDs (`edit/migration.harvesterhci.io.{vmwaresource,ovasource,openstacksource,virtualmachineimport}.vue`). Decision (2026-10-01): **coexist; do not ignore and do not replace.**

- Both UIs read and write the same `migration.harvesterhci.io` CRs, so there is no state to reconcile.
- Positioning: the built-in pages are the "raw resources" view; ours is the guided migration wizard (vCenter explorer, power ops, auto-mapping, plan-filtered logs, support bundle, Forklift).
- CRs we create must keep the exact shape the built-in UI expects (naming, labels, un-slugified `spec.virtualMachineName`), so imports behave identically in either UI.
- Do not duplicate CRUD that the built-in form already does well; link out to it instead.
- Cross-link each source/import in our UI to its native Harvester resource page ("View in Harvester").
- Long-term: a Rancher UI-extension build target for our wizard would remove the overlap and share Harvester's auth (see Phase 4 evaluation).
- Open check: where the built-in pages appear in the Harvester menu, and whether they depend on the controller add-on being enabled. Verify before implementing the cross-links.

## 5. Decisions (2026-10-01)

1. **Distribution:** `harvester/experimental-addons`. Note what that repo actually holds: only an `Addon` manifest (see `harvester-vm-dhcp-controller`) pointing at a chart in `https://charts.harvesterhci.io`, i.e. the chart itself lives in **harvester/charts**. So Phase 5 needs a conversation with the Harvester maintainers (open an issue/enhancement first) about where the chart and images are hosted. Until accepted, Phase 5a (self-hosted chart repo + Addon YAML) is the working channel.
2. **Forklift:** optional engine, UI elements appear only when Forklift is detected (CRDs served and `ForkliftController` Ready).
3. **VM Export (OVA):** the user's premise was that it depends on Forklift tools. Code check says it does not: no Forklift references in `pkg/export*.go`/`ova.go`/`ovf.go`, and `qemu-img` ships in the UI's own image. Only the *round-trip re-import test* uses Forklift's OVA provider. Decision: gate it behind `export.enabled` (default off) and, as a UX choice, show the Export page only when Forklift is detected, since re-importing the OVA is its main use here. Revisit if you want it always available.
4. **Repo:** `doccaz/harvester-migration-addon` (created, public).

## 6. Phased plan

### Phase 0 — Groundwork (≈1 week)
- Create repo skeleton (not on GitHub until you say so): `charts/`, `ui-backend/`, `ui-frontend/`, `docs/`, `.github/`.
- Read `reference/forklift-packaging` to record the Forklift deploy contract (operator chart, `ForkliftController` CR, cert-manager) in `docs/contract-forklift.md`.
- Read the real `harvester/charts` `harvester-vm-import-controller` chart (done: Deployment `Recreate`, `/tmp` emptyDir or PVC, Service 8080, root, affinity away from control-plane) and record its contract in `docs/contract.md`.
- Decide Phase 5 target. Capture current vm-import-ui behaviour as a regression baseline (its `docs/test-log-2026-09-30.md` is a good start).

### Phase 1 — One chart, one Addon (≈1–2 weeks)
- Umbrella chart `harvester-migration` with the upstream controller chart as a dependency (`fullnameOverride: harvester-vm-import-controller`, same labels) and the UI as a second Deployment.
- `Addon` manifest in the style of `experimental-addons/harvester-vm-dhcp-controller` (`addon.harvesterhci.io/experimental: "true"`, `enabled: false`).
- **Conflict guard:** pre-install check (Helm `lookup`) that fails with a clear message if the built-in `vm-import-controller` Addon is enabled; document the migration path (disable old, enable new, CRs are preserved because CRDs and CRs are unchanged).
- Exit criteria: enable the add-on on the lab cluster, import one VMware VM via VMIC, disable → resources gone, CRs intact.

### Phase 2 — Make the UI safe to ship (≈2 weeks) — DONE in v0.2.0
Delivered: per-user token auth (default), TLS verification, RBAC derived from the code, non-root read-only pod, services/proxy access Role, release pipeline. Verification and the known gap (no real limited-user login on Harvester) are in docs/phase2-verification.md. Findings that changed the design: the pod never learns the caller's identity from the proxy; Rancher's `/v3/tokens` needs `Accept: application/json` from browsers; Harvester's Rancher has no global roles.
- Drop NodePort default; serve through the Harvester/Rancher service proxy (NavLink already exists) with `ClusterIP`.
- Authorisation: use the caller's identity (forwarded token / `Impersonate-*` or SubjectAccessReview per request) instead of a god-mode ServiceAccount; narrow RBAC to what each engine needs; split export RBAC behind `export.enabled`.
- Secrets handling review (create/delete of credentials), run as non-root, read-only root FS where possible, NetworkPolicy.
- Exit criteria: a user with only namespace-scoped rights cannot see or act on other namespaces.

### Phase 3 — Backend alignment (≈2–3 weeks) — IN PROGRESS
Rule: no user-visible behaviour change. The REST API that `App.js` calls is the contract, protected at every step by four gates, each step is its own commit with CI green: (1) `testdata/routes.golden` (every method+path), (2) golangci-lint v2.12.2 with the controller's config at 0 issues, (3) unit tests, (4) a read-only snapshot of the 32 safe GET routes on the lab compared with a baseline (`hack/run-snapshot.sh`, `hack/snapshot-api.py`; two runs of identical code diff clean). Details and observations: `docs/refactor-notes.md`.

Done (all verified by the four gates):
- Safety net: route golden test, snapshot tooling.
- Lint adopted from the controller; 36 findings fixed (two added timeouts on calls that could hang: HTTP header read, OVA inventory proxy).
- `handlers.go` (2383 lines) and `forklift_handlers.go` split by area, code motion only (declarations proven byte-identical).
- Packages extracted under `ui-backend/internal/`: `httpx` (response helpers), `kube` (clients, token provider/`Scoped`, TLS policy, GVRs, unstructured helpers), `inventory` (VM tree types + Harvester inventory).

Remaining, in dependency order (leaf first), one commit each: `vcenter` → `capabilities` → `harvester` (namespaces, NADs, storage classes, VMs, generic GETs) → `vmic` (sources, plans, vCenter ops) → `forklift` → `export` (export, job, worker, cleanup, ova, ovf) → `supportbundle` → `api` (routes). `main.go` keeps the `export-worker` / `export-cleanup` command modes (Export Jobs invoke the binary by those arguments).

Changed from the original plan:
- **No import of the controller's `pkg/apis`.** Its `go.mod` pins `k8s.io/client-go v12.0.0+incompatible` and depends on a `replace` block that consumers do not inherit. Typed VMIC objects are instead produced locally with `runtime.DefaultUnstructuredConverter`, with a contract test against the real `virtualmachineimports.migration.harvesterhci.io` CRD saved from the lab.
- **Engine interface only where the engines really share behaviour** (detection/capabilities, plan list and status, logs). VMIC (sources + VirtualMachineImport) and Forklift (provider, inventory service, maps, plan, migration) differ too much for an upfront CreatePlan/RunPlan abstraction; routes stay as they are until Phase 4.
- **Dependency bumps** (k8s 0.28 → current, govmomi 0.33 → 0.52, Go directive) are a separate optional last step; govmomi across 19 minors is where `vcenter.go` is most likely to break. If the `go` directive moves, move the Dockerfile's `golang:` tag in the same commit.
- `logrus.WithFields` is adopted opportunistically in code being moved, not as a sweep.

Lessons and open items:
- Two ~66 MB build binaries were committed by mistake when the backend was imported (removed from the tree; no credentials found). They remain in git history; rewriting history would change every SHA after `d218b1b` and move the release tags, so it is left to a deliberate decision.
- The extraction script initially did not persist removals, which left dead duplicates (found and removed in the `kube` step). The stale-name grep and `unused` lint are part of every step now.
- Existing inconsistency, deliberately not changed: `GET …/{ns}/{name}/yaml` returns 500 for a missing object while the detail routes return 404.
- No release tag is needed for this phase until the full set of gates is clean after the last extraction.

### Phase 4 — Frontend modularisation (≈3 weeks)
- Break up `App.js` (5.7k lines) into engine modules and shared components; engine registry driven by `GET /api/v1/capabilities`.
- Add "View in Harvester" cross-links from sources and imports to the built-in resource pages (see §4b); verify the target routes first.
- Keep `utils.js` and the fixture-replay harness; add component tests per module.
- Forklift section appears only when the operator CRDs are present and a `ForkliftController` is Ready; otherwise a setup checklist (see §4) replaces it.
- Evaluate (not commit to) a Rancher UI-extension build target for the same modules.

### Phase 5 — Distribution (variable, mostly waiting on others)
- **5a Self-hosted:** chart repo on GitHub Pages / OCI + ready-to-apply Addon YAML. Works as soon as Phase 1 is done.
- **5b experimental-addons:** PR adding the Addon entry, with a repo `harvester/charts`-style chart published at charts.harvesterhci.io (their release process applies; DCO sign-off required).
- **5c Bundled:** only if the Harvester team wants it. Requires harvester/charts, harvester/addons (`rancherd-22-addons.yaml`), harvester-installer changes, and probably a decision on whether this *replaces* `vm-import-controller`. Raise as an issue/enhancement first.

### Phase 6 — Hardening and docs
- Upgrade tests (Harvester 1.7 → 1.8), air-gapped image list, resource sizing doc, VDDK guidance, support-bundle integration, release automation (one tag → images + chart).

## 7. Risks

- Upstream controller chart/labels change under us → pin and test against each Harvester minor.
- Dual-controller conflict during migration (Phase 1 guard).
- Forklift packaging is experimental and heavy (Ansible operator, ~12 images, cert-manager, `ForkliftController` CR, VDDK image) and pinned to one upstream release — keep it a separate, optional add-on and detect it rather than bundle it.
- Refactor regressions: mitigated by the four Phase 3 gates; the lab snapshot only covers GET routes, so write paths rely on the unit tests and the route golden.
- Scope: UI + controller + Forklift + export is four products; Phase 5b/5c acceptance is the main uncertainty, which is why A (no fork) comes first.
