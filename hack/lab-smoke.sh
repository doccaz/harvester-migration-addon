#!/usr/bin/env bash
# Smoke-test a deployed harvester-migration UI pod through a port-forward, using the
# kubeconfig user's token (per-user auth). Exercises the paths a read-only snapshot
# cannot: writes, status mapping, the support bundle. It creates a few resources named
# v*-smoke-* and removes them again (also on failure).
#
#   KUBECONFIG=... hack/lab-smoke.sh                  # full run
#   READONLY=1 KUBECONFIG=... hack/lab-smoke.sh       # reads and status checks only
#
# Env: EXPECT_VERSION (the version in addon/harvester-migration.yaml)  APP_NS (harvester-system)  SVC (mig-harvester-migration-ui)
#      FL_NS (forklift)  SRC_NS (default)  PORT (18081)
set -uo pipefail

EXPECT="${EXPECT_VERSION:-$(sed -n 's/^  version: *//p' "$(dirname "$0")/../addon/harvester-migration.yaml" | head -1)}"; [ -n "$EXPECT" ] || { echo "cannot read the expected version; set EXPECT_VERSION"; exit 2; }; APP_NS="${APP_NS:-harvester-system}"; SVC="${SVC:-mig-harvester-migration-ui}"
FL_NS="${FL_NS:-forklift}"; SRC_NS="${SRC_NS:-default}"; PORT="${PORT:-18081}"; READONLY="${READONLY:-0}"
BASE="http://localhost:$PORT"; TMP="$(mktemp -d)"
TOKEN="$(kubectl config view --raw --minify -o jsonpath='{.users[0].user.token}')"
[ -n "$TOKEN" ] || { echo "kubeconfig user has no bearer token"; exit 1; }
pass=0; failn=0; CODE=""; WROTE=0   # set just before the first write, so cleanup only runs if something may exist

cleanup() {
  [ -n "${PF:-}" ] && kill "$PF" 2>/dev/null || true
  if [ "$READONLY" != "1" ] && [ "$WROTE" = 1 ]; then
    kubectl -n "$FL_NS" delete providers.forklift.konveyor.io v-smoke-ova v-smoke-vs --ignore-not-found >/dev/null 2>&1
    kubectl -n "$FL_NS" delete secret v-smoke-ova-secret v-smoke-vs-secret --ignore-not-found >/dev/null 2>&1
    kubectl -n "$SRC_NS" delete vmwaresources.migration.harvesterhci.io v-smoke-src --ignore-not-found >/dev/null 2>&1
    kubectl -n "$SRC_NS" delete secret v-smoke-src-credentials --ignore-not-found >/dev/null 2>&1
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT

ok()   { pass=$((pass+1)); echo "  PASS  $1"; }
bad()  { failn=$((failn+1)); echo "  FAIL  $1"; }
call() { # METHOD PATH [JSON] -> CODE, BODY file
  local m="$1" p="$2" b="${3:-}"
  local args=(-s -o "$TMP/body" -w '%{http_code}' -X "$m" -H "X-Migration-Token: $TOKEN")
  [ -n "$b" ] && args+=(-H 'Content-Type: application/json' -d "$b")
  CODE="$(curl "${args[@]}" "$BASE$p")"
}
expect() { # description wanted-status
  if [ "$CODE" = "$2" ]; then ok "$1 -> $CODE"; else bad "$1 -> $CODE (wanted $2): $(head -c 180 "$TMP/body")"; fi
}
contains() { # description needle haystack
  if grep -qF -- "$2" <<<"$3"; then ok "$1"; else bad "$1 (missing '$2')"; fi
}

echo "== Deployment"
echo "  cluster: $(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}' 2>/dev/null) (KUBECONFIG=${KUBECONFIG:-default})"
if ! ERR="$(kubectl -n "${APP_NS:-harvester-system}" get ns -o name 2>&1 | head -c 300)" || [ -z "$ERR" ]; then
  echo "cannot reach the cluster; set KUBECONFIG to the lab's kubeconfig"; exit 1
fi
case "$ERR" in *"Unable to connect"*|*"no such host"*|*"dial tcp"*|*"error:"*) echo "cannot reach the cluster: $ERR"; echo "set KUBECONFIG to the lab's kubeconfig, e.g. /home/erico/Projetos/local-harvester.yaml"; exit 1;; esac
kubectl -n "$APP_NS" get deploy "$SVC" >/dev/null 2>&1 || { echo "deployment $APP_NS/$SVC not found"; exit 1; }
kubectl -n "$APP_NS" rollout status "deploy/$SVC" --timeout=120s >/dev/null 2>&1 && ok "pod is ready" || bad "pod is not ready"
LOGS="$(kubectl -n "$APP_NS" logs "deploy/$SVC" 2>/dev/null)"
contains "backend reports version v$EXPECT" "Starting VM Import UI Backend v$EXPECT" "$LOGS"
contains "token auth is enabled" "User token auth enabled" "$LOGS"
kubectl -n "$APP_NS" port-forward "svc/$SVC" "$PORT:8080" >/dev/null 2>&1 & PF=$!
curl -s --retry 20 --retry-connrefused --retry-delay 1 --retry-all-errors -o /dev/null "$BASE/" || { echo "port-forward failed"; exit 1; }

echo "== Authentication"
CODE="$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/v1/plans")";                             expect "API without a token" 401
CODE="$(curl -s -o /dev/null -w '%{http_code}' -H 'X-Migration-Token: bogus' "$BASE/api/v1/plans")"; expect "API with a bad token" 401
CODE="$(curl -s -o /dev/null -w '%{http_code}' "$BASE/")";                                         expect "the page itself, no token" 200

echo "== Reads"
for p in capabilities harvester/namespaces harvester/storageclasses harvester/vlanconfigs harvester/vmwaresources harvester/ovasources \
         plans forklift/providers forklift/plans forklift/availability harvester/inventory exports; do
  call GET "/api/v1/$p"; expect "GET /api/v1/$p" 200
done
call GET /api/v1/capabilities; contains "capabilities reports the Harvester version" harvesterVersion "$(cat "$TMP/body")"
contains "capabilities reports each engine's state" '"engines"' "$(cat "$TMP/body")"
for e in vmic forklift export; do
  contains "capabilities has a state for engine $e" "\"$e\":{\"available\"" "$(cat "$TMP/body")"
done
call GET "/api/v1/forklift/availability?namespace=$FL_NS"; contains "Forklift availability names its state" '"state"' "$(cat "$TMP/body")"
call GET /api/v1/harvester/inventory; if grep -q '"warnings"' "$TMP/body"; then bad "inventory is complete (it reports warnings: $(python3 -c "import json;print(json.load(open('$TMP/body')).get('warnings'))" 2>/dev/null))"; else ok "inventory is complete (no warnings)"; fi

echo "== Status mapping (a failing call keeps its meaning)"
call GET "/api/v1/plans/$SRC_NS/nope/yaml";                        expect "missing VMIC plan YAML" 404
call GET "/api/v1/forklift/plans/$FL_NS/nope/yaml";                expect "missing Forklift plan YAML" 404
call GET "/api/v1/exports/$SRC_NS/nope";                           expect "missing export" 404
call POST /api/v1/harvester/namespaces "{\"name\":\"$SRC_NS\"}";   expect "creating a namespace that exists" 409
call GET "/api/v1/forklift/inventory/ova/Bad.NS/x/vms";            expect "OVA inventory with an invalid namespace" 400
call GET "/api/v1/forklift/inventory/ova/$FL_NS/x/secrets";        expect "OVA inventory with an unsupported resource" 400

echo "== Support bundle"
curl -s -o "$TMP/bundle.tgz" -w '%{http_code}' -H "X-Migration-Token: $TOKEN" "$BASE/api/v1/support-bundle" >"$TMP/code"; CODE="$(cat "$TMP/code")"; expect "download" 200
N="$(tar tzf "$TMP/bundle.tgz" 2>/dev/null | wc -l)"; [ "$N" -ge 10 ] && ok "bundle has $N members" || bad "bundle has only $N members"
META="$(tar xzOf "$TMP/bundle.tgz" --wildcards '*/meta.json' 2>/dev/null)"
contains "bundle meta.json records version $EXPECT" "\"appVersion\": \"$EXPECT\"" "$META"
SECRETS="$(tar xzOf "$TMP/bundle.tgz" --wildcards '*/secrets/*' 2>/dev/null)"
if grep -qiE '"(password|token)"[[:space:]]*:[[:space:]]*"[^"]' <<<"$SECRETS"; then bad "bundle leaks a secret value"; else ok "bundle carries no secret values"; fi

echo "== Export"
RUNNING="$(kubectl get vmi -A --no-headers 2>/dev/null | head -1 | awk '{print $1" "$2}')"
if [ -n "$RUNNING" ]; then
  read -r VNS VNAME <<<"$RUNNING"
  call POST /api/v1/exports "{\"namespace\":\"$VNS\",\"name\":\"$VNAME\"}"
  case "$CODE" in
    409) ok "a RUNNING VM ($VNS/$VNAME) is refused -> 409 (the safety check)";;
    503) ok "export is not configured on this deployment -> 503 (enable export.* to test the running-VM refusal)";;
    *)   bad "export of a running VM -> $CODE: $(head -c 180 "$TMP/body")";;
  esac
  JOBS="$(kubectl get jobs -A -l 'vm-import-ui.harvesterhci.io/export=true' --no-headers 2>/dev/null | wc -l)"
  [ "$JOBS" -eq 0 ] && ok "no export Job was created" || echo "  note  $JOBS export Job(s) exist (not necessarily from this run)"
else
  echo "  skip  no running VM found"
fi

if [ "$READONLY" = "1" ]; then echo; echo "(READONLY=1: write checks skipped)"; echo "passed: $pass  failed: $failn"; [ "$failn" -eq 0 ]; exit; fi

WROTE=1
echo "== Writes: Forklift providers (API group and annotation must survive)"
call POST /api/v1/forklift/providers "{\"name\":\"v-smoke-ova\",\"namespace\":\"$FL_NS\",\"url\":\"10.255.255.1:/smoke\",\"providerType\":\"ova\"}"; expect "create OVA provider" 201
call POST /api/v1/forklift/providers "{\"name\":\"v-smoke-vs\",\"namespace\":\"$FL_NS\",\"url\":\"https://vc.invalid/sdk\",\"username\":\"u\",\"password\":\"p\",\"providerType\":\"vsphere\"}"; expect "create vSphere provider" 201
API="$(kubectl -n "$FL_NS" get providers.forklift.konveyor.io v-smoke-vs -o jsonpath='{.apiVersion}' 2>/dev/null)"
[ "$API" = "forklift.konveyor.io/v1beta1" ] && ok "stored apiVersion is $API" || bad "stored apiVersion is '$API'"
ANN="$(kubectl -n "$FL_NS" get providers.forklift.konveyor.io v-smoke-vs -o jsonpath='{.metadata.annotations.forklift\.konveyor\.io/empty-vddk-init-image}' 2>/dev/null)"
[ "$ANN" = "yes" ] && ok "empty-vddk-init-image annotation is set" || bad "annotation missing or wrong ('$ANN')"
kubectl -n "$FL_NS" get secret v-smoke-vs-secret >/dev/null 2>&1 && ok "credentials secret created" || bad "credentials secret missing"
call POST /api/v1/forklift/providers "{\"name\":\"v-smoke-vs\",\"namespace\":\"$FL_NS\",\"url\":\"https://vc.invalid/sdk\",\"username\":\"u\",\"password\":\"p\",\"providerType\":\"vsphere\"}"; expect "create the same provider again" 409
call GET "/api/v1/forklift/providers/$FL_NS/v-smoke-vs/yaml"; expect "provider YAML" 200
call DELETE "/api/v1/forklift/providers/$FL_NS/v-smoke-vs";  expect "delete vSphere provider" 204
call DELETE "/api/v1/forklift/providers/$FL_NS/v-smoke-ova"; expect "delete OVA provider" 204
kubectl -n "$FL_NS" get providers.forklift.konveyor.io v-smoke-vs >/dev/null 2>&1 && bad "provider still exists" || ok "provider removed"
kubectl -n "$FL_NS" get secret v-smoke-vs-secret >/dev/null 2>&1 && bad "provider secret left behind" || ok "provider secret removed"
call DELETE "/api/v1/forklift/providers/$FL_NS/v-smoke-vs"; expect "delete a provider that is gone" 404

echo "== Writes: VMware source"
SRC="{\"name\":\"v-smoke-src\",\"namespace\":\"$SRC_NS\",\"endpoint\":\"https://vc.invalid/sdk\",\"datacenter\":\"DC\",\"username\":\"u\",\"password\":\"p\"}"
call POST /api/v1/harvester/vmwaresources "$SRC"; expect "create source" 201
call GET "/api/v1/harvester/vmwaresources/$SRC_NS/v-smoke-src"; expect "get source" 200
call POST /api/v1/harvester/vmwaresources "$SRC"; expect "create the same source again" 409
call DELETE "/api/v1/harvester/vmwaresources/$SRC_NS/v-smoke-src"; expect "delete source" 204
kubectl -n "$SRC_NS" get secret v-smoke-src-credentials >/dev/null 2>&1 && bad "source secret left behind" || ok "source secret removed"
call DELETE "/api/v1/harvester/vmwaresources/$SRC_NS/v-smoke-src"; expect "delete a source that is gone" 404

echo; echo "passed: $pass  failed: $failn"; [ "$failn" -eq 0 ]
