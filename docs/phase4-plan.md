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
3. A declaration-identity check for moves: `node ui-frontend/tools/check-moves.js <base-ref>` proves every
   top-level declaration of `App.js` at the base ref still exists, textually identical (whitespace-insensitive,
   ignoring the `export` keyword). Moves are made with `node ui-frontend/tools/js-move.js <from> <to> <Names>`,
   which copies the text verbatim, writes exactly the imports needed and drops the origin's unused imports.
4. Lab pass by hand before a release (menu entry, each tab, export, download).

## Steps
- **4.0 Safety net** (done): the snapshot suite and fixtures above.
- **4.1 Shared leaves** (done): `CopyButton`, `DownloadButton`, `SortableHeader`, `Header`, `SubTab`,
  `getNestedValue` moved to `src/shared/` (45/45 declarations identical, 21/21 snapshots unchanged).
- **4.2 Move components by feature** (done, no logic change): `src/inventory/` (tree, details panel, explorer),
  `src/wizard/` (CreatePlanWizard), `src/engines/vmic/` (plans, sources, OVA sources, wizards, details),
  `src/engines/forklift/`, `src/export/`, `src/support/`, `src/about/`. `App.js` went from 5,724 to 804
  lines; 45/45 declarations identical to the pre-Phase-4 `App.js` (`check-moves.js 8ac96d2`), 21/21
  snapshots unchanged.
- **4.3 Split `App()`**
  - **4.3a Hooks** (done, behaviour-identical): the engines' state and handlers moved verbatim into
    `engines/vmic/useVmic.js`, `engines/forklift/useForklift.js`, `hooks/useCapabilities.js`, with the
    sorting helpers in `shared/sorting.js`. `App()` went from 780 to ~400 lines and keeps navigation,
    selection, the polling effect and `renderPage`. Gated by the 21 snapshots plus 24 behaviour tests
    (`App.flows.test.js`: delete/edit/run flows, auto-refresh), written and mutation-checked *before* the move.
  - **4.3b Engine registry and views** (done, behaviour-identical): `engines/registry.js` lists the engines
    (`vmic`, `forklift`), each with a view for the plans, vCenter-sources and OVA-sources pages;
    `engines/EnginePage.js` builds the sub-tabs from the registry. The two copies of the Forklift provider
    JSX became one parametrised `ProvidersView`. `App.js` is now ~315 lines. 28 behaviour tests and the 21
    snapshots gate it; three tests added for the merged view, each mutation-checked.
  - **4.3c Behaviour fixes, with tests** (done; tests written first and seen failing): the app asked for every
    endpoint twice at start-up because the polling effect also depended on `forkliftAvailable` and the expanded
    rows; every background refresh swapped the plans table for "Loading plans..."; an emptied interval field
    started a zero-delay polling loop. Now: one start-up load, a polling tick that reads the latest state through
    a ref (no timer restart when the auto-refresh switch or Forklift availability changes), loading text only
    on the first load, and no polling when the period is not a positive number. Side effect, deliberate:
    expanding a row no longer refetches.
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
