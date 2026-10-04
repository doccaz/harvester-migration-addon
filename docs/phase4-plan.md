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
- **4.4 Capabilities from the backend** (done 2026-10-04, tests first, every new branch mutation-checked):
  - New `internal/engines` package: `vmic`, `forklift` and `export` each report `available`, `not-installed`
    (the CRD is not served: a 404 that names no object), `not-ready` (installed, but no `host` Provider yet),
    `forbidden`, `disabled` or `unknown`, with a message that says what to do. "Absent" and "could not tell"
    are no longer the same answer.
  - `GET /api/v1/capabilities` gains `engines` (reported even when the version cannot be read), and
    `GET /api/v1/forklift/availability` gains `state` and uses the same code; its old fields are unchanged.
  - `inventory.PVCIndex` and the VMI listing return their errors. The inventory answers 200 with `warnings` on
    the root ("Could not list PersistentVolumeClaims (forbidden...)"), which the export page now shows; VMs stay
    blocked from export when run state is unknown. Export preview/create answer with the API's own status
    (403) instead of the misleading 422 "check that its claim exists".
  - `hack/lab-smoke.sh` checks the new fields on the lab.
- **4.5 "View in Harvester" links** (done 2026-10-04): the details pages of vCenter sources, OVA sources and
  VMIC plans link to Harvester's own page for the object. Verified on the lab: the built-in list pages are
  `<origin>/dashboard/c/local/explorer/migration.harvesterhci.io.<vmwaresource|ovasource|virtualmachineimport>`
  (the 4 CRDs `vmwaresources`, `ovasources`, `openstacksources`, `virtualmachineimports` are served and
  have no menu entries of their own, only these Cluster Explorer pages). The object page follows the dashboard's
  usual `.../<type>/<namespace>/<name>`; **to confirm on the lab by opening one object and comparing the URL**.
  The dashboard address is derived from where this UI was loaded (`/k8s/clusters/<id>/...` proxy path, any
  prefix kept); served any other way there is no link and the page is unchanged (`shared/harvesterLinks.js`).
- **4.6 Setup checklist** (done 2026-10-04, tests first): `ForkliftUnavailable` now acts on the backend's reason.
  *Not installed*: the three setup steps from docs/contract-forklift.md (cert-manager, the `forklift-operator`
  add-on, a `ForkliftController`, with an example). *Installed but not ready*: the first two steps shown as done,
  the third as the one left. *No permission* and *could not check*: the cause and what to do, no setup steps (the
  cluster may be fine). With no state (an older backend) the previous message renders unchanged. Also fixed: a
  Retry with a changed namespace checked twice, because the mount effect depended on a callback that changes with
  the namespace.
- **4.7 Component tests per module**; short evaluation of a Rancher UI-extension build target (not a commitment).
- Release as v0.5.0 after a lab pass.

## Findings from the characterization work
- **OVA sources without credentials were unusable here** (fixed 2026-10-04, tests first). The details page
  read `source.spec.credentials.namespace` unguarded and crashed, and the backend answered 500 ("OvaSource
  missing credentials secret name") on Get and Update, so such a source could not even be opened for editing.
  Our own API always creates sources with a credentials secret, but one made with `kubectl` or Harvester's own
  UI may name none. Now: Get returns the source (no username), Update changes the URL/timeout and, only if the
  request supplies a username or password, creates `<name>-ova-credentials` (or reuses a leftover one) and links
  it, and the details pages show "none". The vCenter details page got the same guard.
- The app refetches around start-up and flips back to "Loading..." while it does; tests wait for a quiet
  network (`settle`) before a snapshot. The duplicate start-up fetches are worth removing in 4.3.

## Findings from the lab pass of v0.5.0-rc1 (2026-10-04)
- **Refresh looked dead.** The 4.3c fix removed the "Loading plans..." flash, which had been the only visible sign
  of a refresh. The plans footer now shows "Updated HH:MM:SS" (it moves on every poll and on "Refresh Now").
  A failed refresh used to *empty* the table (`setPlans([])`); it now keeps the table and shows "Refresh failed:
  <reason> (showing data from HH:MM:SS)". A failed first load still shows an empty table.
- **Forklift objects get "View in Harvester" too.** Harvester has no page of its own for them, only the dashboard's
  YAML editor, so the link (provider and plan details) is `.../explorer/forklift.konveyor.io.<provider|plan>/<ns>/<name>?mode=edit&as=yaml`,
  labelled "(YAML)". URL shape taken from a real dashboard address.
- **Confirmed on the lab:** the "View in Harvester" links for vCenter sources, OVA sources and plans land on the
  right pages (so the assumed object URL was right); an OVA source created without `credentials` is accepted by the
  CRD and its details page shows "none".
