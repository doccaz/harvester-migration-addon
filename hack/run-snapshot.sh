#!/usr/bin/env bash
# Build the backend, run it in token mode against a cluster, snapshot every GET
# route into OUTDIR (see snapshot-api.py), then stop it. Read-only.
#   KUBECONFIG=... hack/run-snapshot.sh OUTDIR
# Uses the kubeconfig's bearer token and its server as the Rancher API:
#   KUBE_API_URL (default: <kubeconfig server>/k8s/clusters/local)
set -euo pipefail
OUT="${1:?usage: run-snapshot.sh OUTDIR}"
HERE="$(cd "$(dirname "$0")" && pwd)"
BIN="$(mktemp -d)/ui-backend"

(cd "$HERE/../ui-backend" && go build -o "$BIN" .)
SERVER="$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}' | sed 's#/*$##')"
export MIGRATION_TOKEN="$(kubectl config view --raw --minify -o jsonpath='{.users[0].user.token}')"
[ -n "$MIGRATION_TOKEN" ] || { echo "kubeconfig user has no bearer token"; exit 1; }

USER_AUTH=token KUBE_API_URL="${KUBE_API_URL:-$SERVER/k8s/clusters/local}" INSECURE_SKIP_TLS_VERIFY=true \
  UI_PATH=/tmp LOG_LEVEL=warn "$BIN" >"$BIN.log" 2>&1 &
PID=$!
trap 'kill $PID 2>/dev/null || true' EXIT
curl -s --retry 20 --retry-connrefused --retry-delay 1 --retry-all-errors -o /dev/null http://localhost:8080/

python3 "$HERE/snapshot-api.py" snap http://localhost:8080 "$OUT"
