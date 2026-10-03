#!/usr/bin/env bash
# Download a finished export through the deployed UI's own download path (ticket ->
# serve pod -> streaming proxy), interrupting the transfer on purpose to prove that a
# download resumes. Creates one short-lived serve pod in the export's namespace (it
# exits by itself after 30 minutes idle, or when the export is deleted).
#
#   KUBECONFIG=... hack/lab-download.sh                    # the newest downloadable export
#   EXPORT_NS=labs EXPORT_ID=0123abcd... hack/lab-download.sh
#   VIA=proxy hack/lab-download.sh    # go through the Kubernetes API server's service
#                                     # proxy (what the dashboard uses), not a port-forward
#
# Env: EXPORT_NS, EXPORT_ID (default: newest)  OUT (default: ./lab-download.ova)
#      CUT seconds before each deliberate interruption (default 15)  INTERRUPTS (default 2)
#      EXPECT_SHA256 (compare the result)  VERIFY=1 (also run hack/verify-ova.sh)
#      APP_NS (harvester-system)  SVC (mig-harvester-migration-ui)  PORT (18083)
set -uo pipefail
APP_NS="${APP_NS:-harvester-system}"; SVC="${SVC:-mig-harvester-migration-ui}"; PORT="${PORT:-18083}"
OUT="${OUT:-./lab-download.ova}"; CUT="${CUT:-15}"; INTERRUPTS="${INTERRUPTS:-2}"; VIA="${VIA:-forward}"
HERE="$(cd "$(dirname "$0")" && pwd)"; TMP="$(mktemp -d)"
TOKEN="$(kubectl config view --raw --minify -o jsonpath='{.users[0].user.token}')"
[ -n "$TOKEN" ] || { echo "kubeconfig user has no bearer token"; exit 1; }
cleanup() { [ -n "${PF:-}" ] && kill "$PF" 2>/dev/null; rm -rf "$TMP"; true; }
trap cleanup EXIT

echo "== Cluster: $(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}' 2>/dev/null) (KUBECONFIG=${KUBECONFIG:-default})"
kubectl -n "$APP_NS" get svc "$SVC" >/dev/null 2>&1 || { echo "service $APP_NS/$SVC not found (is KUBECONFIG the lab's?)"; exit 1; }

if [ "$VIA" = proxy ]; then
  SERVER="$(kubectl config view --raw --minify -o jsonpath='{.clusters[0].cluster.server}')"
  BASE="$SERVER/api/v1/namespaces/$APP_NS/services/http:$SVC:8080/proxy"
  CURL=(curl -sk -H "Authorization: Bearer $TOKEN")
  echo "== Via the API server service proxy: $BASE"
else
  kubectl -n "$APP_NS" port-forward "svc/$SVC" "$PORT:8080" >/dev/null 2>&1 & PF=$!
  BASE="http://localhost:$PORT"; CURL=(curl -s)
  curl -s --retry 20 --retry-connrefused --retry-delay 1 --retry-all-errors -o /dev/null "$BASE/" || { echo "port-forward failed"; exit 1; }
  echo "== Via a port-forward to the UI pod"
fi
api() { "${CURL[@]}" -o "$TMP/body" -w '%{http_code}' -H "X-Migration-Token: $TOKEN" "${@:2}" "$BASE$1"; }

echo "== Choosing the export"
CODE="$(api /api/v1/exports)"; [ "$CODE" = 200 ] || { echo "  list failed: HTTP $CODE $(head -c 300 "$TMP/body")"; exit 1; }
PICK="$(EXPORT_NS="${EXPORT_NS:-}" EXPORT_ID="${EXPORT_ID:-}" python3 - "$TMP/body" <<'PY'
import json, os, sys
d = json.load(open(sys.argv[1]))
items = d if isinstance(d, list) else d.get("exports", d.get("items", []))
ns, id_ = os.environ["EXPORT_NS"], os.environ["EXPORT_ID"]
ok = [e for e in items if e.get("downloadable") and (not ns or e["namespace"] == ns) and (not id_ or e["exportId"] == id_)]
ok.sort(key=lambda e: e.get("createdAt", ""), reverse=True)
if ok:
    e = ok[0]; print(e["namespace"], e["exportId"], e.get("targetName") or e.get("vmName"), e.get("sizeBytes", 0))
PY
)"
[ -n "$PICK" ] || { echo "  no downloadable export found (finished, with a target)"; exit 1; }
read -r NS ID NAME SIZE <<<"$PICK"; echo "  $NS/$ID ($NAME.ova, ${SIZE:-0} bytes known)"

ticket() {
  local start=$SECONDS code
  while :; do
    code="$(api "/api/v1/exports/$NS/$ID/download-ticket" -X POST)"
    case "$code" in
      200) python3 -c "import json; print(json.load(open('$TMP/body'))['url'])"; echo "  ticket after $((SECONDS-start))s" >&2; return 0;;
      202) [ $((SECONDS-start)) -gt 240 ] && { echo "  serve pod not ready after 240s" >&2; return 1; }; sleep 3;;
      *)   echo "  ticket failed: HTTP $code $(head -c 300 "$TMP/body")" >&2; return 1;;
    esac
  done
}
echo "== Asking for a download ticket (the serve pod starts on the first call)"
URL="$(ticket)" || exit 1
kubectl -n "$NS" get pod "vm-export-serve-$ID" --no-headers 2>/dev/null | sed 's/^/  /'

echo "== Downloading to $OUT, interrupting every ${CUT}s, $INTERRUPTS time(s), then resuming"
PART="$OUT.part"; rm -f "$PART"; N=0; START=$SECONDS
while :; do
  if [ "$N" -lt "$INTERRUPTS" ]; then LIM=(--max-time "$CUT"); else LIM=(); fi
  "${CURL[@]}" -f -C - -o "$PART" "${LIM[@]}" "$BASE$URL"; RC=$?
  HAVE="$(stat -c %s "$PART" 2>/dev/null || echo 0)"
  if [ "$RC" = 0 ]; then echo "  complete: $HAVE bytes in $((SECONDS-START))s"; break; fi
  if [ "$RC" = 28 ] && [ "$N" -lt "$INTERRUPTS" ]; then N=$((N+1)); echo "  deliberately stopped at $HAVE bytes; resuming ($N)"; continue; fi
  if [ "$RC" = 22 ]; then echo "  HTTP error at $HAVE bytes; getting a new ticket"; URL="$(ticket)" || exit 1; continue; fi
  echo "  curl failed (exit $RC) at $HAVE bytes; retrying"; sleep 3
done
mv "$PART" "$OUT"
GOT="$(sha256sum "$OUT" | cut -d' ' -f1)"; echo "== SHA-256 $GOT"
if [ -n "${EXPECT_SHA256:-}" ]; then
  [ "$GOT" = "$EXPECT_SHA256" ] && echo "  PASS  matches EXPECT_SHA256" || { echo "  FAIL  expected $EXPECT_SHA256"; exit 1; }
fi
if [ "${VERIFY:-0}" = 1 ]; then "$HERE/verify-ova.sh" "$OUT" || exit 1; fi
echo "== Done. The serve pod exits after 30 min idle (or delete the export). Check: kubectl -n $NS get pod vm-export-serve-$ID"
