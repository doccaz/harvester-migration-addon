#!/usr/bin/env bash
# Checks the UI's proxy access Role against the real kube-apiserver by
# impersonating users. Run ON A HARVESTER NODE as root: Rancher's proxy ignores
# --as, so this must use the node's own admin kubeconfig.
#
# Prerequisite: the chart installed with  --set 'ui.access.users={migtest}'
# Read-only: impersonated GETs, authorisation checks, and a few requests to the
# UI service that are rejected or answered by the UI without changing anything.
set -uo pipefail
export KUBECONFIG="${KUBECONFIG:-/etc/rancher/rke2/rke2.yaml}"
NS="${NS:-harvester-system}"
SVC="${SVC:-mig-harvester-migration-ui}"
PORT="${PORT:-8080}"
ALLOWED="${ALLOWED:-migtest}"
DENIED="${DENIED:-nobody}"

pass=0; fail=0
check() { # description expected(allow|deny) actual
  if [ "$2" = "$3" ]; then echo "  PASS  $1 -> $3"; pass=$((pass+1)); else echo "  FAIL  $1 -> got $3, expected $2"; fail=$((fail+1)); fi
}
# A forbidden request says so; anything else (200, 404, 405...) means authorisation passed.
verdict() { grep -qi "forbidden" <<<"$1" && echo deny || echo allow; }

echo "== Role and binding present"
kubectl -n "$NS" get role "$SVC-access" -o name 2>&1 | sed 's/^/  /'
kubectl -n "$NS" get rolebinding "$SVC-access" -o jsonpath='  subjects: {.subjects[*].kind}/{.subjects[*].name}{"\n"}' 2>&1

echo; echo "== GET through the service proxy, by name spelling (user $ALLOWED should pass for the spelling Rancher uses)"
for name in "$SVC" "$SVC:$PORT" "http:$SVC:$PORT" "http:$SVC:http"; do
  out="$(kubectl get --raw "/api/v1/namespaces/$NS/services/$name/proxy/" --as="$ALLOWED" 2>&1 | head -c 200)"
  echo "  $ALLOWED  $name -> $(verdict "$out")"
done

echo; echo "== Expected results"
URL="/api/v1/namespaces/$NS/services/http:$SVC:$PORT/proxy/"
check "$ALLOWED may GET the UI"      allow "$(verdict "$(kubectl get --raw "$URL" --as="$ALLOWED" 2>&1 | head -c 200)")"
check "$DENIED must not GET the UI"  deny  "$(verdict "$(kubectl get --raw "$URL" --as="$DENIED" 2>&1 | head -c 200)")"
# Verbs: POST=create, PUT=update, DELETE=delete. The UI answers or rejects these
# harmlessly (nothing under "/" accepts writes).
check "$ALLOWED may POST (create)"   allow "$(verdict "$(echo '{}' | kubectl create --raw "$URL" -f - --as="$ALLOWED" 2>&1 | head -c 200)")"
check "$DENIED must not POST"        deny  "$(verdict "$(echo '{}' | kubectl create --raw "$URL" -f - --as="$DENIED" 2>&1 | head -c 200)")"
check "$ALLOWED may DELETE"          allow "$(verdict "$(kubectl delete --raw "$URL" --as="$ALLOWED" 2>&1 | head -c 200)")"
check "$DENIED must not DELETE"      deny  "$(verdict "$(kubectl delete --raw "$URL" --as="$DENIED" 2>&1 | head -c 200)")"
# The Role must not open any other service in the namespace.
OTHER="$(kubectl -n "$NS" get svc -o name | grep -v "$SVC" | head -1 | cut -d/ -f2)"
if [ -n "$OTHER" ]; then
  check "$ALLOWED must not reach $OTHER" deny "$(verdict "$(kubectl get --raw "/api/v1/namespaces/$NS/services/$OTHER/proxy/" --as="$ALLOWED" 2>&1 | head -c 200)")"
fi

echo; echo "== What $ALLOWED can do on the resources themselves (should be 'no': access to the page is not access to data)"
for q in "list secrets -A" "get secrets -n default" "create vmwaresources.migration.harvesterhci.io -n default" "create virtualmachineimports.migration.harvesterhci.io -n default" "create providers.forklift.konveyor.io -n default"; do
  check "$ALLOWED can-i $q" deny "$(kubectl auth can-i $q --as="$ALLOWED" 2>&1 | grep -q '^yes' && echo allow || echo deny)"
done

echo; echo "passed: $pass  failed: $fail"
[ "$fail" -eq 0 ]
