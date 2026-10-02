# harvester-migration-addon

A single Harvester add-on that ships the VM Import Controller and a migration UI.
Forklift is an optional engine, shown only when it is detected on the cluster.

Status: usable on Harvester 1.8 for evaluation (see [PLAN.md](PLAN.md)). Not yet
reviewed by the Harvester team.

## Install (Helm)

    helm repo add mig https://doccaz.github.io/harvester-migration-addon
    helm install mig mig/harvester-migration -n harvester-system

- If Harvester's built-in `vm-import-controller` add-on is enabled, the install is
  refused (two controllers would fight over the same resources). Disable that
  add-on first, or install UI-only with `--set controller.enabled=false`.
- The UI authorises every call with the signed-in user's own Rancher token
  (`ui.auth.mode=token`, the default). Cluster admins can open it from the
  *VM Migration* menu entry; grant others access with `ui.access.users` /
  `ui.access.groups`. Details: [docs/identity.md](docs/identity.md).
- Forklift is a separate add-on ([harvester/forklift-packaging](https://github.com/harvester/forklift-packaging));
  it is detected, never installed here.

More: [docs/contract.md](docs/contract.md), [docs/rbac-audit.md](docs/rbac-audit.md),
[docs/phase2-verification.md](docs/phase2-verification.md).
