#!/usr/bin/env bash
# Export a STOPPED Harvester VM to an OVA through the deployed UI's API, watch the Job,
# and optionally fetch the file. Writes to the cluster: a ReadWriteMany export volume in
# the VM's namespace (first use) and one Job.
#
#   KUBECONFIG=... VM_NS=labs VM_NAME=bastion-galins-server hack/lab-export.sh
#   FETCH=1 ... hack/lab-export.sh        # also copy the OVA to the current directory
#
# Env: VM_NS, VM_NAME (required)  PROFILE (vmware|portable|faithful, default vmware)
#      APP_NS (harvester-system)  SVC (mig-harvester-migration-ui)  PORT (18082)
#      TIMEOUT seconds (3600)     EXPORT_PVC (mig-harvester-migration-exports)
set -uo pipefail
: "${VM_NS:?set VM_NS}"; : "${VM_NAME:?set VM_NAME}"
PROFILE="${PROFILE:-vmware}"; APP_NS="${APP_NS:-harvester-system}"; SVC="${SVC:-mig-harvester-migration-ui}"
PORT="${PORT:-18082}"; TIMEOUT="${TIMEOUT:-3600}"; EXPORT_PVC="${EXPORT_PVC:-mig-harvester-migration-exports}"
BASE="http://localhost:$PORT"; TMP="$(mktemp -d)"
TOKEN="$(kubectl config view --raw --minify -o jsonpath='{.users[0].user.token}')"
[ -n "$TOKEN" ] || { echo "kubeconfig user has no bearer token"; exit 1; }
cleanup() { kill "${PF:-0}" 2>/dev/null || true; kubectl -n "$VM_NS" delete pod ova-fetch --ignore-not-found --wait=false >/dev/null 2>&1; rm -rf "$TMP"; }
trap cleanup EXIT
api() { curl -s -o "$TMP/body" -w '%{http_code}' -H "X-Migration-Token: $TOKEN" -H 'Content-Type: application/json' "${@:2}" "$BASE$1"; }
field() { python3 -c "import sys,json; print(json.load(open('$TMP/body')).get('$1',''))" 2>/dev/null; }

echo "== Pre-flight"
kubectl -n "$VM_NS" get vm "$VM_NAME" >/dev/null 2>&1 || { echo "VM $VM_NS/$VM_NAME not found"; exit 1; }
if kubectl -n "$VM_NS" get vmi "$VM_NAME" >/dev/null 2>&1; then echo "VM is RUNNING; power it off first (the export refuses a running VM, correctly)."; exit 1; fi
echo "  VM $VM_NS/$VM_NAME is stopped"
kubectl -n "$APP_NS" port-forward "svc/$SVC" "$PORT:8080" >/dev/null 2>&1 & PF=$!
curl -s --retry 20 --retry-connrefused --retry-delay 1 --retry-all-errors -o /dev/null "$BASE/" || { echo "port-forward failed"; exit 1; }

echo "== Preview (builds the OVF descriptor without touching the cluster)"
CODE="$(api /api/v1/exports/preview -X POST -d "{\"namespace\":\"$VM_NS\",\"name\":\"$VM_NAME\",\"profile\":\"$PROFILE\"}")"
echo "  HTTP $CODE; $(wc -c <"$TMP/body") bytes"; [ "$CODE" = 200 ] || { head -c 400 "$TMP/body"; echo; exit 1; }

echo "== Create the export (profile $PROFILE)"
CODE="$(api /api/v1/exports -X POST -d "{\"namespace\":\"$VM_NS\",\"name\":\"$VM_NAME\",\"profile\":\"$PROFILE\"}")"
if [ "$CODE" != 202 ]; then echo "  HTTP $CODE: $(head -c 400 "$TMP/body")"; exit 1; fi
ID="$(field exportId)"; JOB="$(field jobName)"; TARGET="$(field target)"
echo "  accepted: id=$ID job=$JOB target=$TARGET"

echo "== Watching the Job (timeout ${TIMEOUT}s)"
jobfield() { kubectl -n "$VM_NS" get job "$JOB" -o "jsonpath={.status.$1}" 2>/dev/null; }
START=$SECONDS; LAST=""; RESULT=""
while :; do
  ACTIVE="$(jobfield active)"; SUCC="$(jobfield succeeded)"; FAILED="$(jobfield failed)"
  POD="$(kubectl -n "$VM_NS" get pods -l "vm-import-ui.harvesterhci.io/export-id=$ID" --no-headers 2>/dev/null | awk '{print $3}' | head -1)"
  LINE="active=${ACTIVE:-0} succeeded=${SUCC:-0} failed=${FAILED:-0} pod=${POD:-none}"
  [ "$LINE" != "$LAST" ] && echo "  [$((SECONDS-START))s] $LINE" && LAST="$LINE"
  if [ "${SUCC:-0}" -ge 1 ]; then RESULT=ok; break; fi
  if [ "${FAILED:-0}" -ge 1 ]; then RESULT=failed; break; fi
  if [ $((SECONDS-START)) -gt "$TIMEOUT" ]; then RESULT=timeout; break; fi
  sleep 10
done
api "/api/v1/exports/$VM_NS/$ID" >/dev/null; echo "  API view: $(head -c 300 "$TMP/body")"
if [ "$RESULT" != ok ]; then
  echo "== Export did not succeed ($RESULT). Worker log:"
  kubectl -n "$VM_NS" logs -l "vm-import-ui.harvesterhci.io/export-id=$ID" --tail=60 2>&1 | head -80
  exit 1
fi
echo "  Job succeeded after $((SECONDS-START))s"

echo "== Where the OVA is"
echo "  volume : $VM_NS/$EXPORT_PVC (RWX), file: /$TARGET   (the UI pod cannot serve this namespace's volume)"
if [ "${FETCH:-0}" = 1 ]; then
  echo "== Fetching with a temporary read-only pod"
  kubectl -n "$VM_NS" apply -f - >/dev/null <<YAML
apiVersion: v1
kind: Pod
metadata: {name: ova-fetch, namespace: $VM_NS}
spec:
  restartPolicy: Never
  containers:
  - {name: c, image: registry.suse.com/bci/bci-base:latest, command: [sleep, "7200"], securityContext: {runAsUser: 0}, volumeMounts: [{name: v, mountPath: /export, readOnly: true}]}
  volumes: [{name: v, persistentVolumeClaim: {claimName: $EXPORT_PVC, readOnly: true}}]
YAML
  kubectl -n "$VM_NS" wait --for=condition=Ready pod/ova-fetch --timeout=180s >/dev/null && kubectl -n "$VM_NS" exec ova-fetch -- ls -la /export
  kubectl cp "$VM_NS/ova-fetch:/export/$TARGET" "./$TARGET" && ls -la "./$TARGET" && echo "  next: hack/verify-ova.sh ./$TARGET"
else
  cat <<TXT
  To copy it out (a read-only helper pod; delete it afterwards):
    kubectl -n $VM_NS apply -f - <<'YAML'
    apiVersion: v1
    kind: Pod
    metadata: {name: ova-fetch}
    spec:
      restartPolicy: Never
      containers:
      - {name: c, image: registry.suse.com/bci/bci-base:latest, command: [sleep, "7200"], securityContext: {runAsUser: 0}, volumeMounts: [{name: v, mountPath: /export, readOnly: true}]}
      volumes: [{name: v, persistentVolumeClaim: {claimName: $EXPORT_PVC, readOnly: true}}]
    YAML
    kubectl -n $VM_NS wait --for=condition=Ready pod/ova-fetch --timeout=180s
    kubectl cp $VM_NS/ova-fetch:/export/$TARGET ./$TARGET
    kubectl -n $VM_NS delete pod ova-fetch
    hack/verify-ova.sh ./$TARGET
TXT
fi
