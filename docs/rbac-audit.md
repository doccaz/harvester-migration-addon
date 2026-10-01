# UI RBAC audit (Phase 2)

Method: every `Clientset.*` and `Dynamic.Resource(...)` call in `ui-backend/*.go`
(non-test) was enumerated (2026-10-01) and mapped to a rule in
`charts/harvester-migration/templates/ui-rbac.yaml`.

Removed from the inherited vm-import-ui ClusterRole (never called by the code):
`nodes`, `events`, `configmaps`, `persistentvolumes`, `apps/*`, CDI
(`datavolumes`, `datasources`), `harvesterhci.io/*` wildcard (only `settings` get
is used), `virtualmachineinstancemigrations`, `network.harvesterhci.io/vlanconfigs`
(the handler actually lists NetworkAttachmentDefinitions), all `watch` verbs, and
write access to `kubevirt.io` resources (the UI only reads VMs).

Secrets are the sensitive part: only get/create/update/delete by name, no
list/watch. Kubernetes RBAC cannot restrict by namespace in a ClusterRole, so the
UI ServiceAccount can still read any Secret whose name it knows: this is why the
per-user identity work (docs/identity.md) is required before calling the UI safe.

Gated by chart values: `engines.vmic`, `engines.forklift`, `export.enabled`
(jobs + PVC create).
