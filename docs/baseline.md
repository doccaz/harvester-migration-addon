# Regression baseline (Phase 0)

Behaviour to preserve while refactoring. Reference: vm-import-ui
`docs/test-log-2026-09-30.md` and `frontend/src/__fixtures__/bundles/`.

- [ ] VMIC: create VmwareSource (+secret), build a plan, run, see status/logs.
- [ ] VMIC: OVA source plan.
- [ ] vCenter explorer: inventory, power ops, rename, MAC edit.
- [ ] Forklift: provider, plan (NetworkMap + StorageMap + Plan atomically), run/cancel/delete.
- [ ] Support bundle: redaction and scoping rules.
- [ ] Export: running VM blocked; OVA validates against DSP8023.
- [ ] `virtualMachineName` is never slugified; only `metadata.name` is.
