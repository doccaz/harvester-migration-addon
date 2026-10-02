# Phase 3 refactor notes

## Safety net
- `ui-backend/testdata/routes.golden`: every (method, path); `go test` fails if a
  route is dropped or renamed. Regenerate deliberately with
  `go test -run TestRoutesAreStable -update`.
- `hack/run-snapshot.sh OUTDIR` + `hack/snapshot-api.py diff A B`: read-only
  snapshot of the 32 GET routes that are safe to call, against a live cluster.
  Run it before a change and after, then diff. Two runs of identical code must
  diff clean (they do). Snapshots hold real object names: keep them out of git.

## Observations (not changed: Phase 3 must not alter behaviour)
- `GET .../{namespace}/{name}/yaml` returns **500** when the object does not exist,
  while the matching detail route returns 404 (`HandleGetSourceYAML`,
  `HandleGetForkliftPlanYAML`). Candidate fix, with an explicit behaviour note.

## Decisions
- Do NOT import `github.com/harvester/vm-import-controller` for typed objects: its
  `go.mod` pins `k8s.io/client-go v12.0.0+incompatible` and relies on a `replace`
  block (client-go/api/apimachinery -> 0.33.7) that consumers do not inherit.
  Typing is done locally with `runtime.DefaultUnstructuredConverter` against the
  CRD schema instead.
