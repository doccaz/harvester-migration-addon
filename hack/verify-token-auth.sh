#!/usr/bin/env bash
# Verifies the assumptions behind USER_AUTH=token (docs/identity.md).
# Creates one 10-minute Rancher token and one short-lived pod in "default";
# both are removed on exit. The token value is never printed.
set -uo pipefail

KUBECONFIG="${KUBECONFIG:-$HOME/.kube/config}"
export KUBECONFIG
VIP="${VIP:-$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}' | sed 's#/*$##')}"
T="$(kubectl config view --raw --minify -o jsonpath='{.users[0].user.token}')"
[ -n "$T" ] || { echo "kubeconfig user has no bearer token"; exit 1; }

TOKEN_ID=""
cleanup() {
  [ -n "$TOKEN_ID" ] && curl -sk -X DELETE -H "Authorization: Bearer $T" "$VIP/v3/tokens/$TOKEN_ID" -o /dev/null -w "cleanup: token $TOKEN_ID deleted (HTTP %{http_code})\n"
  kubectl -n default delete pod tokprobe --ignore-not-found --wait=false >/dev/null 2>&1
}
trap cleanup EXIT

echo "== 1. Mint a token (POST $VIP/v3/tokens, ttl 600000)"
RESP="$(curl -sk -X POST -H "Authorization: Bearer $T" -H 'Content-Type: application/json' \
  "$VIP/v3/tokens" -d '{"type":"token","description":"harvester-migration-ui probe","ttl":600000}')"
MINTED="$(printf '%s' "$RESP" | python3 -c '
import sys, json
try:
    d = json.load(sys.stdin)
except Exception as e:
    print("::parse-error::" + str(e)); sys.exit()
open("/dev/stderr", "w").write(json.dumps({k: d.get(k) for k in ("id","ttl","expired","expiresAt","type","code","message")}) + "\n")
print(d.get("token", ""))
')"
TOKEN_ID="$(printf '%s' "$RESP" | python3 -c 'import sys,json; print(json.load(sys.stdin).get("id",""))' 2>/dev/null)"
[ -n "$MINTED" ] && [[ "$MINTED" != ::parse-error::* ]] || { echo "no token minted; stopping"; exit 1; }
echo "token minted: yes (length ${#MINTED}, id $TOKEN_ID)"

echo; echo "== 1b. Minted token works at the VIP"
curl -sk -o /dev/null -w "HTTP %{http_code}\n" -H "Authorization: Bearer $MINTED" "$VIP/k8s/clusters/local/api/v1/namespaces/default"

echo; echo "== 2. Same token from inside the cluster"
kubectl -n default delete pod tokprobe --ignore-not-found >/dev/null 2>&1
kubectl -n default run tokprobe --restart=Never --image=registry.suse.com/bci/bci-base:latest \
  --env="TOKEN=$MINTED" --command -- sh -c '
R=https://rancher.cattle-system.svc
echo "rancher svc, -k:        $(curl -sk -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $TOKEN" $R/k8s/clusters/local/api/v1/namespaces/default)"
echo "rancher svc, verified:  $(curl -s  -o /dev/null -w "%{http_code} (curl exit %{exitcode})" -H "Authorization: Bearer $TOKEN" $R/k8s/clusters/local/api/v1/namespaces/default 2>&1)"
echo "kube-apiserver svc, -k: $(curl -sk -o /dev/null -w "%{http_code}" -H "Authorization: Bearer $TOKEN" https://kubernetes.default.svc/api/v1/namespaces/default)  (401 expected: Rancher tokens are Rancher-only)"
echo "--- certificate presented by rancher.cattle-system.svc"
echo | openssl s_client -connect rancher.cattle-system.svc:443 -servername rancher.cattle-system.svc -showcerts 2>/dev/null | grep -E "^ *[0-9] s:|^ *i:|Verification" || echo "(openssl not available)"
' >/dev/null
for i in $(seq 1 40); do
  phase="$(kubectl -n default get pod tokprobe -o jsonpath='{.status.phase}' 2>/dev/null)"
  [ "$phase" = "Succeeded" ] || [ "$phase" = "Failed" ] && break; sleep 3
done
kubectl -n default logs tokprobe 2>&1 | sed "s/$MINTED/<token>/g"

echo; echo "== 3. Browser same-origin minting: paste this in the DevTools console of the Harvester UI tab:"
cat <<'JS'
fetch('/v3/tokens',{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json','X-Api-Csrf':(document.cookie.split('; ').find(c=>c.startsWith('CSRF='))||'CSRF=').slice(5)},body:JSON.stringify({type:'token',description:'probe-browser',ttl:600000})}).then(async r=>{const b=await r.json();console.log('status',r.status,'id',b.id,'ttl',b.ttl,'hasToken',!!b.token)})
JS
echo "(then delete 'probe-browser' under your avatar > Account & API Keys)"
