#!/usr/bin/env bash
# Copy a large OVA off an export volume reliably, and verify it.
#
# `kubectl cp` streams a tar through the exec channel and drops on multi-GB files. This
# fetches fixed-size chunks with dd (each retried, and skipped if already present, so a
# re-run resumes), reassembles them, and compares the SHA-256 computed INSIDE the cluster
# with the local one. It first verifies the OVA in place (member order, manifest digests)
# by streaming hack/verify-ova.sh into the helper pod, so integrity is known before any
# transfer.
#
#   VM_NS=labs hack/fetch-ova.sh bastion-galins-server-c0e85f41797f.ova [outdir]
#
# Env: VM_NS (required)  PVC (mig-harvester-migration-exports)  CHUNK_MB (256)
#      RETRIES (6)  KEEP_POD=1 to leave the helper pod  VERIFY_IN_CLUSTER=0 to skip
#      HELPER_IMAGE (registry.suse.com/bci/bci-base:latest). It needs bash, tar, sha256sum, sed,
#      grep, cut, tr, df, stat, dd. No awk needed. The UI image (ghcr.io/doccaz/harvester-
#      migration-ui:<tag>) also works and adds qemu-img; bci-busybox does not (no bash).
set -uo pipefail
NS="${VM_NS:?set VM_NS}"; FILE="${1:?usage: fetch-ova.sh <file-on-volume> [outdir]}"; OUT="${2:-.}"
HELPER_IMAGE="${HELPER_IMAGE:-registry.suse.com/bci/bci-base:latest}"
PVC="${PVC:-mig-harvester-migration-exports}"; CHUNK_MB="${CHUNK_MB:-256}"; RETRIES="${RETRIES:-6}"; POD=ova-fetch
HERE="$(cd "$(dirname "$0")" && pwd)"
K() { kubectl -n "$NS" "$@"; }
OWN=0
cleanup() { [ "$OWN" = 1 ] && [ "${KEEP_POD:-0}" != 1 ] && K delete pod "$POD" --ignore-not-found --wait=false >/dev/null 2>&1; true; }
trap cleanup EXIT
mkdir -p "$OUT"

# Say which cluster this will touch, and check it can be read, before creating anything.
echo "== Cluster: $(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}' 2>/dev/null) (KUBECONFIG=${KUBECONFIG:-default})"
if ! ERR="$(K get pvc "$PVC" -o name 2>&1)"; then
  echo "cannot read pvc $NS/$PVC:"; echo "  $(echo "$ERR" | head -c 300)"
  echo "wrong cluster? set KUBECONFIG to the lab's kubeconfig, e.g. KUBECONFIG=/home/erico/Projetos/local-harvester.yaml"
  exit 1
fi

# A helper pod left over from an earlier run may still be terminating: wait for it to go.
if K get pod "$POD" -o jsonpath='{.metadata.deletionTimestamp}' 2>/dev/null | grep -q .; then
  echo "== Waiting for the previous helper pod to finish terminating"
  K wait --for=delete "pod/$POD" --timeout=120s >/dev/null 2>&1
fi
if ! K get pod "$POD" >/dev/null 2>&1; then
  echo "== Starting a read-only helper pod on $NS/$PVC"
  K apply -f - >/dev/null <<YAML || exit 1
apiVersion: v1
kind: Pod
metadata: {name: $POD, namespace: $NS}
spec:
  restartPolicy: Never
  containers:
  - {name: c, image: $HELPER_IMAGE, command: [sleep, "7200"], securityContext: {runAsUser: 0}, volumeMounts: [{name: v, mountPath: /export, readOnly: true}]}
  volumes: [{name: v, persistentVolumeClaim: {claimName: $PVC, readOnly: true}}]
YAML
  OWN=1
  K wait --for=condition=Ready "pod/$POD" --timeout=180s >/dev/null || { echo "helper pod did not become ready"; exit 1; }
else
  OWN=1   # adopt a helper pod left by an earlier run of this script
fi

SIZE="$(K exec "$POD" -- stat -c %s "/export/$FILE" 2>/dev/null)"
[ -n "$SIZE" ] || { echo "/export/$FILE not found on the volume"; K exec "$POD" -- ls -la /export; exit 1; }
echo "== $FILE: $SIZE bytes ($((SIZE/1048576)) MiB)"

if [ "${VERIFY_IN_CLUSTER:-1}" = 1 ]; then
  echo "== Verifying in place (inside the cluster; nothing is transferred)"
  K exec -i "$POD" -- bash -s -- "/export/$FILE" < "$HERE/verify-ova.sh" || { echo "in-place verification FAILED; not fetching"; exit 1; }
fi

echo "== SHA-256 of the file on the volume"
WANT="$(K exec "$POD" -- sha256sum "/export/$FILE" | awk '{print $1}')"; echo "  $WANT"
[ -n "$WANT" ] || { echo "could not hash the remote file"; exit 1; }

CHUNK=$((CHUNK_MB*1048576)); N=$(( (SIZE + CHUNK - 1) / CHUNK ))
PARTS="$OUT/.$FILE.parts"; mkdir -p "$PARTS"
echo "== Fetching $N chunk(s) of up to ${CHUNK_MB} MiB (resumable)"
for ((i=0; i<N; i++)); do
  OFF=$((i*CHUNK)); EXP=$(( SIZE-OFF < CHUNK ? SIZE-OFF : CHUNK )); P="$PARTS/$(printf '%05d' $i)"
  if [ -f "$P" ] && [ "$(stat -c %s "$P")" = "$EXP" ]; then echo "  chunk $((i+1))/$N already present"; continue; fi
  ok=0
  for ((t=1; t<=RETRIES; t++)); do
    K exec "$POD" -- dd if="/export/$FILE" bs=1M skip=$((i*CHUNK_MB)) count="$CHUNK_MB" status=none > "$P.tmp" 2>/dev/null
    if [ "$(stat -c %s "$P.tmp" 2>/dev/null || echo 0)" = "$EXP" ]; then mv "$P.tmp" "$P"; ok=1; break; fi
    echo "  chunk $((i+1))/$N attempt $t failed ($(stat -c %s "$P.tmp" 2>/dev/null || echo 0)/$EXP bytes); retrying"; sleep 2
  done
  [ "$ok" = 1 ] || { echo "chunk $((i+1)) failed after $RETRIES attempts; re-run to resume"; exit 1; }
  echo "  chunk $((i+1))/$N ok"
done

echo "== Reassembling"
cat "$PARTS"/[0-9]* > "$OUT/$FILE" || exit 1
GOT="$(sha256sum "$OUT/$FILE" | awk '{print $1}')"
if [ "$GOT" = "$WANT" ]; then echo "  PASS  local SHA-256 matches the volume's ($GOT)"; rm -rf "$PARTS"; else echo "  FAIL  local $GOT != volume $WANT (kept $PARTS)"; exit 1; fi
ls -la "$OUT/$FILE"; echo "next: $HERE/verify-ova.sh $OUT/$FILE   (full check incl. schema and disk; needs free space ~ the disk size in \$TMPDIR)"
