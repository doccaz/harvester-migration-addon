# Moving from the built-in `vm-import-controller` add-on

CRDs and CRs are created by the controller at runtime and survive add-on removal.

1. Note running imports; wait for them to finish.
2. Disable `Addon/vm-import-controller` in `harvester-system`.
3. Enable `Addon/harvester-migration`.

Already happy with the built-in controller? Install with
`controller.enabled=false` for a UI-only deployment.
