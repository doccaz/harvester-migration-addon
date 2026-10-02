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

## Observations (still open)
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
