# Contract with the Forklift add-on

Source: https://github.com/harvester/forklift-packaging (validated by them against Forklift v2.9.2).

Forklift is **detected, never installed** by this add-on.

## Deploy sequence (theirs)
1. `cert-manager` present in the cluster.
2. Addon `forklift-operator` (chart `forklift-operator`, namespace `forklift`):
   an Ansible operator plus the `forklift.konveyor.io` CRDs.
3. A `ForkliftController` CR (e.g. `spec.feature_ui_plugin: "false"`) makes the
   operator create the controller, API, validation, inventory, virt-v2v,
   populators and OVA provider server (~12 images).

## Detection the UI should do (Phase 4 `GET /api/v1/capabilities`)
| Check | Meaning |
|---|---|
| CRDs `providers/plans/networkmaps/storagemaps/migrations.forklift.konveyor.io` served | operator installed |
| `ForkliftController` exists and is Ready | stack running |
| cert-manager CRDs (`certificates.cert-manager.io`) | prerequisite met |
| controller image tag / CR status | reported Forklift version |

Forklift modules (and the Export page, see PLAN.md §5) appear only when all
checks pass; otherwise a setup checklist is shown.
