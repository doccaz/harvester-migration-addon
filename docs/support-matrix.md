# Support matrix

The add-on bundles the upstream VM Import Controller as a Helm subchart. Harvester ships its own
controller per release and tests it against that release's CRDs and APIs, so the bundled controller
must come from the **same Harvester minor** as the cluster.

| Add-on chart line | Bundled controller | Harvester it targets | Verified on |
|---|---|---|---|
| 0.3.1, 0.4.x | `harvester-vm-import-controller` 1.8.2 (`rancher/harvester-vm-import-controller:v1.8.2`) | 1.8.x | Harvester 1.8.2 (lab) |
| 0.3.0 | 1.9.0 | 1.9.x (mistakenly released for 1.8) | do not use on 1.8; `controller.enabled=false` was the only safe setting |
| (planned) | 1.9.x | 1.9.x | not yet |

- The UI itself is not tied to a Harvester minor: it detects capabilities at runtime.
- On a cluster whose minor differs from the bundled controller, the install notes print a warning
  (`hack/check-version-warning.sh` tests it). It never blocks an install: dev builds report versions
  such as `master-head`, which are skipped.
- To use the built-in add-on's controller instead, set `controller.enabled=false`.
- To override the version the warning compares against (for example when the `server-version` Setting
  is unreadable), set `controller.clusterVersionOverride`.

When a new Harvester minor ships: bump the subchart dependency to the matching `harvester-vm-import-controller`
release, update this table, run the lab smoke test, and cut a new chart release.
