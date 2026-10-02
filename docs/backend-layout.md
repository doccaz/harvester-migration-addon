# Backend layout

`ui-backend` is one Go module with a deliberately thin `main`. Everything else is under
`internal/`, one package per concern, with dependencies pointing one way (leaf first):

```
httpx          JSON response helpers; API-error -> HTTP status mapping (StatusFor)
kube           clients, per-user token Provider and Scoped wrapper, GVRs, unstructured helpers
inventory      VM tree types + the Harvester/KubeVirt VM inventory
vcenter        govmomi access, credentials, typed errors, HTTPStatus
capabilities   Harvester version -> feature flags
harvester      namespaces, NADs, storage classes, VM list, generic resource/YAML GETs
vmic           VM Import Controller engine: plans, VMware/OVA sources, vCenter operations
forklift       Forklift engine: providers, plans, migrations, inventory proxy, logs
export         Harvester VM -> OVA: handlers, Job builders, worker and cleanup entry points
supportbundle  redacted diagnostics tar.gz
api            the router (the only place that knows every route) and panic recovery
testutil       fake clients, table-test helpers, failure injection (tests only)
```

`main.go` does what only `main` can: dispatch the export Job modes (`export-worker`,
`export-cleanup`, see `export.WorkerArg`/`CleanupArg`), set up logging, build the
`kube.Provider`, stamp the release version (`-X main.appVersion`), and serve
`api.NewRouter(...)`.

## Rules that are tested
- **The REST API is the contract with the frontend.** `internal/api/testdata/routes.golden`
  pins every method + path; regenerate deliberately with
  `go test ./internal/api -run TestRoutesAreStable -update`.
- **Every API route requires a user token** in token mode
  (`TestEveryAPIRouteRequiresAUserToken` walks the whole table); the static frontend does not.
- **Errors keep their meaning.** A failing Kubernetes call answers with the API server's
  status (`httpx.RespondWithAPIErrorMsg`); 500 is for failures that are the server's.
  Deliberate 500s are listed in `docs/refactor-notes.md`.
- **The Job entry points and the build stamp are pinned**: Job commands, the Dockerfile's
  install path and `-X main.appVersion`, and the release workflow's `VERSION` argument.

## Changing code safely
`hack/run-snapshot.sh OUT` snapshots every GET route that is safe to call against a live
cluster; `hack/snapshot-api.py diff A B` compares two snapshots. Take one before and after a
change. Move-only refactors should also be checked declaration-by-declaration against
`HEAD` modulo renames (this caught a script that corrupted string literals).
Lint is the controller's golangci config (v2.12.2) and must stay at 0.
