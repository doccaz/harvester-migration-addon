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
#   KUBECONFIG=... VM_NS=labs hack/fetch-ova.sh bastion-galins-server-c0e85f41797f.ova [outdir]
#
# The exec stream can be cut after a while (observed: every failure at 96-99.9% of a 256 MiB
# chunk), so the transfer size ADAPTS: it halves after a failure (down to MIN_MB) and doubles
# again after a few successes (up to CHUNK_MB). Each finished range is kept as a part named
# <start MiB>-<MiB>, so a re-run resumes whatever the earlier run completed.
#
# Env: VM_NS (required)  PVC (mig-harvester-migration-exports)  CHUNK_MB (max range, 128)
#      MIN_MB (smallest range, 8)  RETRIES (consecutive failures at MIN_MB before giving up, 8)  KEEP_POD=1 to leave the helper pod  VERIFY_IN_CLUSTER=0 to skip
#      HELPER_IMAGE (registry.suse.com/bci/bci-base:latest). It needs bash, tar, sha256sum, sed,
#      grep, cut, tr, df, stat, dd. No awk needed. The UI image (ghcr.io/doccaz/harvester-
#      migration-ui:<tag>) also works and adds qemu-img; bci-busybox does not (no bash).
set -uo pipefail
NS="${VM_NS:?set VM_NS}"; FILE="${1:?usage: fetch-ova.sh <file-on-volume> [outdir]}"; OUT="${2:-.}"
HELPER_IMAGE="${HELPER_IMAGE:-registry.suse.com/bci/bci-base:latest}"
PVC="${PVC:-mig-harvester-migration-exports}"; CHUNK_MB="${CHUNK_MB:-128}"; MIN_MB="${MIN_MB:-8}"; RETRIES="${RETRIES:-8}"; POD=ova-fetch
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

TOTAL_MIB=$(( (SIZE + 1048575) / 1048576 )); PARTS="$OUT/.$FILE.parts"; mkdir -p "$PARTS"
echo "== Fetching $TOTAL_MIB MiB in adaptive ranges (start ${CHUNK_MB} MiB, min ${MIN_MB} MiB; resumable)"
# parts from the earlier fixed-size version (00000, 00001, ... of 256 MiB each) become <start>-256
for f in "$PARTS"/[0-9][0-9][0-9][0-9][0-9]; do
  [ -e "$f" ] || continue
  mv "$f" "$PARTS/$(printf '%08d' $(( 10#${f##*/} * 256 )))-256"
done
rm -f "$PARTS"/*.tmp
pos=0; cur="$CHUNK_MB"; streak=0; fails=0; retried=0
while [ "$pos" -lt "$TOTAL_MIB" ]; do
  # a part already fetched at this position (from an earlier run)? skip over it
  have="$(ls "$PARTS"/"$(printf '%08d' "$pos")"-* 2>/dev/null | head -1)"
  if [ -n "$have" ]; then
    n="${have##*-}"; e=$(( (n*1048576) < (SIZE - pos*1048576) ? n*1048576 : SIZE - pos*1048576 ))
    if [ "$(stat -c %s "$have")" = "$e" ]; then pos=$((pos+n)); continue; fi
    rm -f "$have"
  fi
  count=$(( TOTAL_MIB - pos < cur ? TOTAL_MIB - pos : cur ))
  exp=$(( count*1048576 < SIZE - pos*1048576 ? count*1048576 : SIZE - pos*1048576 ))
  P="$PARTS/$(printf '%08d' "$pos")-$count"
  K exec "$POD" -- dd if="/export/$FILE" bs=1M skip="$pos" count="$count" status=none > "$P.tmp" 2>/dev/null
  if [ "$(stat -c %s "$P.tmp" 2>/dev/null || echo 0)" = "$exp" ]; then
    mv "$P.tmp" "$P"; pos=$((pos+count)); fails=0; streak=$((streak+1))
    printf '  %d/%d MiB  (range %d MiB ok)\n' "$pos" "$TOTAL_MIB" "$count"
    if [ "$streak" -ge 4 ] && [ "$cur" -lt "$CHUNK_MB" ]; then cur=$(( cur*2 > CHUNK_MB ? CHUNK_MB : cur*2 )); streak=0; fi
  else
    got="$(stat -c %s "$P.tmp" 2>/dev/null || echo 0)"; rm -f "$P.tmp"; streak=0; retried=$((retried+1))
    if [ "$cur" -gt "$MIN_MB" ]; then
      cur=$(( cur/2 < MIN_MB ? MIN_MB : cur/2 )); fails=0
      echo "  range of $count MiB at ${pos} MiB dropped at $((got/1048576))/$((exp/1048576)) MiB; shrinking to ${cur} MiB"
    else
      fails=$((fails+1)); echo "  range of $count MiB at ${pos} MiB failed (${fails}/${RETRIES} at the minimum size)"
      [ "$fails" -ge "$RETRIES" ] && { echo "giving up at ${pos} MiB; re-run to resume (finished ranges are kept in $PARTS)"; exit 1; }
      sleep 2
    fi
  fi
done
echo "  all ranges fetched ($retried retried)"

echo "== Reassembling"
cat "$PARTS"/[0-9]* > "$OUT/$FILE" || exit 1
GOT="$(sha256sum "$OUT/$FILE" | awk '{print $1}')"
if [ "$GOT" = "$WANT" ]; then echo "  PASS  local SHA-256 matches the volume's ($GOT)"; rm -rf "$PARTS"; else echo "  FAIL  local $GOT != volume $WANT (kept $PARTS)"; exit 1; fi
ls -la "$OUT/$FILE"; echo "next: $HERE/verify-ova.sh $OUT/$FILE   (full check incl. schema and disk; needs free space ~ the disk size in \$TMPDIR)"
