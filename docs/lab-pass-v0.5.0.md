# Lab pass for v0.5.0-rc1

What changed since v0.4.0: the UI download of exported OVAs was already in 0.4.0; new here is the whole
Phase 4 frontend (modules, engine registry, fewer duplicate requests, no "Loading plans..." flicker),
engine states in `/capabilities` and `/forklift/availability`, the Forklift setup checklist, "View in
Harvester" links, export-page warnings, and OVA sources without credentials.

## Upgrade
```
helm repo update doccaz
helm -n harvester-system upgrade mig doccaz/harvester-migration --version 0.5.0-rc1 --reuse-values
kubectl -n harvester-system rollout status deploy/mig-harvester-migration-ui
```
A pre-release is installed only by naming its version; `helm search repo` hides it without `--devel`.

## API smoke test
```
KUBECONFIG=/home/erico/Projetos/local-harvester.yaml EXPECT_VERSION=0.5.0-rc1 ./hack/lab-smoke.sh
```
Expect 55 checks to pass (the new ones: engines in `/capabilities`, a state in `/forklift/availability`,
an inventory without warnings).

## In the browser (hard-refresh first)
| Check | Expect |
|---|---|
| Menu | "VM Migration" opens the UI; every tab loads |
| Plans | the table stays on screen during the 10 s refresh (no "Loading plans..." flash); expanding a row does not reload it |
| Auto-refresh | the switch stops the refresh; emptying the seconds field does not make it hammer the API |
| **View in Harvester** | on a vCenter source, an OVA source and a plan, open Details: the link opens Harvester's page for that object. **Confirm the object URL is `.../explorer/migration.harvesterhci.io.vmwaresource/<namespace>/<name>`** (assumed; tell me if it differs) |
| Forklift present | the Forklift sub-tabs list providers and plans as before |
| Forklift setup checklist | disable the `forklift-operator` add-on (or point Namespace at one without Forklift): the tab shows "Forklift is not installed" with 3 steps; with the operator on but no ForkliftController, "installed but not ready" with 2 steps done |
| Export page | lists VMs; no yellow warning when you can list PVCs; Download works (browser, to the end, SHA-256) |
| OVA source created with kubectl and **no** `credentials` | Details opens, shows "none"; Edit opens |
| Support bundle | still downloads |
| Login/logout | works |

To make the OVA source: `kubectl apply -f -` with `kind: OvaSource`, `spec.url: http://files.lab/exports`
and no `spec.credentials` (delete it afterwards).

## Roll back
```
helm -n harvester-system upgrade mig doccaz/harvester-migration --version 0.4.0 --reuse-values
```
