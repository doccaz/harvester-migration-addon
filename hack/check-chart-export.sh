#!/usr/bin/env bash
# Checks the chart's export-storage behaviour offline: the UI Deployment is replaced (not rolled)
# when export is on, and the install notes warn when the export volume would use a storage class
# that attaches to one node at a time. The warning helper is rendered through a temporary template
# (NOTES.txt is not part of `helm template` output).
#
#   hack/check-chart-export.sh      (needs helm and the chart's dependencies)
set -uo pipefail
CHART="$(cd "$(dirname "$0")/.." && pwd)/charts/harvester-migration"
T="$CHART/templates/zz-export-check.yaml"
trap 'rm -f "$T"' EXIT
printf '# W:{{ include "harvester-migration.exportStorageWarning" . | quote }}\n' > "$T"
FAILS=0
render() { helm template t "$CHART" --set ui.auth.ca.existingSecret=ca "$@" 2>&1; }
warn() { # name want(warn|none) args...
  local name="$1" want="$2"; shift 2; local out got
  out="$(render "$@" -s templates/zz-export-check.yaml)"
  case "$out" in *'# W:""'*) got=none;; *'# W:"'*) got=warn;; *) got="error: $(echo "$out" | head -2 | tr '\n' ' ')";; esac
  if [ "$got" = "$want" ]; then echo "  PASS  $name"; else echo "  FAIL  $name (want $want, got $got)"; FAILS=$((FAILS+1)); fi
}
strategy() { # name want(Recreate|none) args...
  local name="$1" want="$2"; shift 2; local got
  got="$(render "$@" -s templates/ui-deployment.yaml | grep -A1 '^  strategy:' | grep -o 'Recreate' | head -1)"
  [ -z "$got" ] && got=none
  if [ "$got" = "$want" ]; then echo "  PASS  $name"; else echo "  FAIL  $name (want $want, got $got)"; FAILS=$((FAILS+1)); fi
}
echo "== UI Deployment strategy"
strategy "export off: default rolling update"          none
strategy "export on: the pod is replaced"              Recreate --set export.enabled=true
strategy "export on with a custom class: still replaced" Recreate --set export.enabled=true --set export.storage.storageClass=nfs-rwx
echo "== Export storage warning"
warn "export off: no warning"                          none
warn "export on, class left empty (cluster default)"    warn --set export.enabled=true
warn "export on, class harvester-longhorn (migratable)" warn --set export.enabled=true --set export.storage.storageClass=harvester-longhorn
warn "export on, a NFS/RWX class"                       none --set export.enabled=true --set export.storage.storageClass=nfs-rwx
warn "export on, an existing claim"                     none --set export.enabled=true --set export.storage.existingClaim=my-claim
warn "existing claim, class left empty"                 none --set export.enabled=true --set export.storage.existingClaim=my-claim --set export.storage.storageClass=
echo; [ "$FAILS" = 0 ] && echo passed || { echo "$FAILS failed"; exit 1; }
