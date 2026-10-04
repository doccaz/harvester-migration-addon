# Contract with Harvester and the upstream controller chart

Recorded 2026-10-01 from harvester/charts `harvester-vm-import-controller`
(published 1.9.0 and 1.8.2, dev 1.10.0-dev.0; the add-on pins the line that matches the Harvester it targets, currently 1.8.2) and harvester/harvester.

## What Harvester hard-codes
- `pkg/util/constants.go`: `HarvesterVMImportController = "vm-import-controller-harvester-vm-import-controller"`.
- Setting validator (`pkg/webhook/resources/setting/validator.go`): finds the
  controller's volumes by label `app.kubernetes.io/name=harvester-vm-import-controller`
  (storage-network setting changes check them).
- Built-in add-on: `Addon/vm-import-controller` in `harvester-system`, chart
  `harvester-vm-import-controller`, `fullnameOverride: harvester-vm-import-controller`
  (harvester/addons `pkg/templates/rancherd-22-addons.yaml`).

=> Our chart keeps the subchart's `fullnameOverride` and name label unchanged.

## Upstream chart behaviour we rely on
- Deployment, strategy `Recreate`, root, `/tmp` is an emptyDir or a PVC (`pvcClaim.enabled`).
- Service `harvester-vm-import-controller` on 8080 serves converted disks to
  VirtualMachineImages. The UI therefore runs in a separate Pod.
- Resources: 0.5 CPU / 2Gi requests, 2 CPU / 4Gi limits.
- CRDs are created by the controller at runtime (`crd.Create`), not by the chart.
  Removing the add-on keeps CRDs and CRs.

## Conflict guard
`templates/conflict-guard.yaml` fails the install if `Addon/vm-import-controller`
is enabled. Helm's release-ownership check on the identically named
ServiceAccount/Deployment is a second line of defence. Verified against the lab
cluster on 2026-10-01.

## The controller's resources: a machine-checked contract (added 2026-10-04)
The controller creates its CRDs **at runtime from its Go types**, so there is no CRD manifest to pin; the types
at the bundled version are the contract. `ui-backend/internal/vmic/testdata/upstream-contract.json` lists every
spec/status field path of `VmwareSource`, `OvaSource`, `OpenstackSource` and `VirtualMachineImport`, written by

```
git clone --depth 1 --branch v1.8.2 https://github.com/harvester/vm-import-controller reference/vm-import-controller
go run hack/crd-contract/main.go reference/vm-import-controller v1.8.2 > ui-backend/internal/vmic/testdata/upstream-contract.json
```

What checks it:
- `internal/vmic/contract_test.go`: our typed objects, the plan-editor payload and what the source handlers write only
  use fields that exist upstream; the contract's version must equal the controller version in `Chart.yaml`.
- `ui-frontend/src/contract.test.js`: the plan the wizard builds, and every fixture the UI tests render.
- CI regenerates the file from the upstream tag in the chart pin and diffs it (no hand edits, no staleness).

**When bumping the bundled controller** (new Harvester minor): change the pin in `Chart.yaml`, regenerate the
file with the new tag, and fix whatever the tests report (a renamed or removed field is a UI break caught here
instead of on a cluster).

Found by it on its first run: our local `VirtualMachineImportSpec` declared `schedule`, which the controller does
not have; the local status declared `conditions` where the controller's field is `importConditions`; and the UI
fixtures used the wrong names (the UI itself already reads `importConditions || conditions`). `OvaSource.credentials`
is `+optional` upstream, which matches the OVA-without-credentials fix.

