# Phase 4: frontend modularisation

`ui-frontend/src/App.js` is 5.7k lines: ~40 components and one `App()` that owns the state of every
page. Phase 4 splits it into engine modules behind a capabilities-driven registry, without changing
what users see. The backend refactor's lesson applies: a move is only safe with a gate that proves
nothing else changed.

## Gates (every step)
1. **Page snapshots** (`src/App.pages.test.js`, 21 scenarios, `src/__snapshots__/`): every page, subtab,
   details panel, wizard and the export page rendered from `src/testing/fixtures.js` with deterministic
   dates (en-US/UTC) and a settled network. A pure code move must leave them byte-identical; an
   intentional UI change updates them with `yarn test -u` and the diff is reviewed. Each scenario also
   asserts strings that must be on the page, so a snapshot cannot capture a blank or error page.
2. The existing unit tests (`utils`, `apiClient`, `exportDownload`, `App`) and `yarn build`.
3. A declaration-identity check for moves (moved components must be textually identical modulo
   imports/exports), as in the backend.
4. Lab pass by hand before a release (menu entry, each tab, export, download).

## Steps
- **4.0 Safety net** (done): the snapshot suite and fixtures above.
- **4.1 Shared leaves**: `CopyButton`, `DownloadButton`, `SortableHeader`, `Header`, `SubTab`,
  `getNestedValue`, status helpers move to `src/shared/`.
- **4.2 Move components by feature**, no logic change: `engines/vmic/` (plans, sources, OVA sources,
  explorer, wizards), `engines/forklift/`, `export/`, `support/` (support bundle), `about/`.
- **4.3 Split `App()`**: per-engine state and fetching into hooks; an engine registry
  (`{id, label, available(capabilities), pages}`) replaces the hard-coded sub-tab arrays.
- **4.4 Capabilities from the backend**: extend `GET /api/v1/capabilities` with per-engine availability
  and reasons; fix `forklift.CheckAvailability` (any failure reads as "not available") and
  `inventory.PVCIndex` (swallows a failed PVC list) so "unknown/forbidden" is not shown as "absent".
- **4.5 "View in Harvester" links** from sources and imports to Harvester's own pages. Verify the
  dashboard routes on the lab first (open check in PLAN.md section 4b).
- **4.6 Setup checklist** replaces the Forklift pages when Forklift is absent (today a one-line message).
- **4.7 Component tests per module**; short evaluation of a Rancher UI-extension build target (not a commitment).
- Release as v0.5.0 after a lab pass.

## Findings from the characterization work
- **OVA source details crash when `spec.credentials` is absent** (`OvaSourceDetails` reads
  `source.spec.credentials.namespace`). This add-on's own API always creates OVA sources with a
  credentials secret, but a source created with `kubectl` or Harvester's own UI may have none. To be
  fixed in a separate, tested commit after the moves, so the moves stay behaviour-identical.
- The app refetches around start-up and flips back to "Loading..." while it does; tests wait for a quiet
  network (`settle`) before a snapshot. The duplicate start-up fetches are worth removing in 4.3.
