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

- **forklift**: 21 API-call sites keep the API server's status (providers, plans,
  migrations, inventory; message text unchanged), the vCenter inventory goes through
  the shared `vcenter.HTTPStatus` (which `vmic` now uses too), and a missing
  `forklift-inventory` service is 404 (was 500). The 9 sites left at 500 are the same
  deliberate categories as in vmic: malformed stored providers, `SetNested*`, YAML
  marshal and request building. Tests: 19 status rows, simulator-backed inventory
  tests, and the first real tests of the OVA inventory proxy (URL built, port chosen
  from the service, upstream status passed through, 502 when unreachable) via a
  recording transport; mutation-checked.
- The OVA inventory proxy refuses a namespace that is not a DNS label (400) before it
  becomes part of a host name.

## Observations (still open)
- **`CheckAvailability` reports "Forklift not available" for any failure** reading the
  `host` Provider, including a plain permission denial, so a limited user in token mode
  would see Forklift as missing. Not a 500; revisit with the capabilities detection in
  Phase 4 (it should distinguish "absent" from "not allowed to look").
- **Blanket 404s mask permission errors.** Some handlers answer 404 for *any* failure
  of a lookup (`GetResource`, the source detail routes, parts of export and forklift
  providers; about 18 sites: export 8, vmic 6, forklift 3, harvester 1), so a forbidden read looks like "not found". Not a 500,
  so left alone for now; in token mode it should become the API status too.
- **Blanket 500s still to convert as packages move:** export (11), support bundle (2),
  inventory (1). Everything else left at 500 is on purpose (see the vmic and forklift
  entries above). `harvester` is done (its four list calls were
  converted in a follow-up; only the YAML marshal failure stays 500, which is internal).
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
