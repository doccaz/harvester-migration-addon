# Contract with Harvester and the upstream controller chart

Recorded 2026-10-01 from harvester/charts `harvester-vm-import-controller`
(published 1.9.0, dev 1.10.0-dev.0) and harvester/harvester.

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
