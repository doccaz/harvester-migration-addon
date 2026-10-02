# Phase 3 refactor notes

## Safety net
- `ui-backend/testdata/routes.golden`: every (method, path); `go test` fails if a
  route is dropped or renamed. Regenerate deliberately with
  `go test -run TestRoutesAreStable -update`.
- `hack/run-snapshot.sh OUTDIR` + `hack/snapshot-api.py diff A B`: read-only
  snapshot of the 32 GET routes that are safe to call, against a live cluster.
  Run it before a change and after, then diff. Two runs of identical code must
  diff clean (they do). Snapshots hold real object names: keep them out of git.

## Behaviour changes made during Phase 3 (deliberate, each its own commit)
- **API errors keep their status** (`httpx.RespondWithAPIError`): a missing object on
  the four YAML handlers is now 404 (was 500), and creating an existing namespace is
  409 (was 500); a forbidden create is 403. Verified on the lab: exactly six YAML
  routes went 500 -> 404 (forklift migrations/networkmaps/plans/providers/
  storagemaps, ovasources); the other 26 GET routes are unchanged, and the baseline
  was retaken afterwards. The frontend only reads `response.ok` / the body text, so
  it is unaffected. The other ~150 `RespondWithError(..., 500, ...)` sites still map
  every failure to 500; convert them as each package is extracted, with a test each.
- `GetSourceYAML` sends `X-Content-Type-Options: nosniff`.
- HTTP server `ReadHeaderTimeout` (10s) and a 60s timeout on the OVA inventory proxy.

- **vmic**: every Kubernetes API failure in the plan, source and vCenter-operation
  handlers keeps the API server's status (24 sites, `RespondWithAPIErrorMsg`: the
  message text is unchanged, only the status). `GetPlanLogs` now separates "pod list
  failed" (API status) from "no controller pod running" (404). Typed vCenter errors
  (`ErrUnsupportedOperation`, `DeviceNotFoundError`, `IsNotFound`) make an unsupported
  power operation a 400 and an unknown VM, datacenter or device key a 404.
  Tests: 32 status rows for plans/sources (VMware and OVA) plus simulator-backed
  handler tests for inventory, power, rename and MAC; mutation-checked.
- **Deliberately still 500 in vmic** (19 sites): internal conversion, marshal and
  `SetNested*` failures, and a stored source missing `spec.endpoint` or its
  credentials reference (a data problem). Also an **unreachable or refusing vCenter**:
  an upstream failure that would be a 502, kept at 500 because an intermediary
  (ingress, Rancher's proxy) may replace a backend 502 with its own error page and
  hide the message the UI shows. Revisit only after confirming that on the lab.

## Observations (still open)
- **Blanket 404s mask permission errors.** Some handlers answer 404 for *any* failure
  of a lookup (`GetResource`, the source detail routes, parts of export and forklift
  providers; about 19 sites), so a forbidden read looks like "not found". Not a 500,
  so left alone for now; in token mode it should become the API status too.
- **Blanket 500s still to convert as packages move:** export (11), forklift (30),
  harvester (4 list calls: next commit), support bundle (2), inventory (1).
- List routes that return `list.Items` directly could answer `null` instead of `[]`
  for an empty result if a client library returns a nil slice; the tests accept
  either because the frontend only needs an empty list. Not confirmed against a
  real server.

## Decisions
- Do NOT import `github.com/harvester/vm-import-controller` for typed objects: its
  `go.mod` pins `k8s.io/client-go v12.0.0+incompatible` and relies on a `replace`
  block (client-go/api/apimachinery -> 0.33.7) that consumers do not inherit.
  Typing is done locally with `runtime.DefaultUnstructuredConverter` against the
  CRD schema instead.
