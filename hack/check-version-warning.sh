#!/usr/bin/env bash
# Checks the chart's "controller version differs from the cluster's Harvester" warning.
# `helm template` has no cluster to read, so the Harvester version is given with
# controller.clusterVersionOverride, and the helper is rendered through a temporary template
# (NOTES.txt itself is not part of `helm template` output). No cluster access needed.
#
#   hack/check-version-warning.sh        (run from the repo root; needs helm and the chart's dependencies)
set -uo pipefail
CHART="$(cd "$(dirname "$0")/.." && pwd)/charts/harvester-migration"
BUNDLED="$(sed -n '/name: harvester-vm-import-controller/{n;s/^ *version: *//p}' "$CHART/Chart.yaml" | head -1)"
[ -n "$BUNDLED" ] || { echo "cannot read the bundled controller version from Chart.yaml"; exit 2; }
VALUES_VER="$(sed -n 's/^ *bundledVersion: *"\{0,1\}\([^" ]*\).*/\1/p' "$CHART/values.yaml" | head -1)"
MINOR="${BUNDLED%.*}"
T="$CHART/templates/zz-version-warning-check.yaml"
trap 'rm -f "$T"' EXIT
printf '# W:{{ include "harvester-migration.controllerVersionWarning" . | quote }}\n' > "$T"
FAILS=0
render() { helm template t "$CHART" --set ui.auth.ca.existingSecret=ca "$@" -s templates/zz-version-warning-check.yaml 2>&1; }
expect() { # name, want (warn|none), helm args...
  local name="$1" want="$2"; shift 2; local out; out="$(render "$@")"
  local got=none; case "$out" in *'# W:""'*) ;; *'# W:"'*) got=warn;; *) got="error: $(echo "$out" | head -2 | tr '\n' ' ')";; esac
  if [ "$got" = "$want" ]; then echo "  PASS  $name"; else echo "  FAIL  $name (want $want, got $got)"; FAILS=$((FAILS+1)); fi
}
echo "== Bundled controller $BUNDLED"
if [ "$VALUES_VER" = "$BUNDLED" ]; then echo "  PASS  values.yaml controller.bundledVersion ($VALUES_VER) matches Chart.yaml"
else echo "  FAIL  values.yaml controller.bundledVersion is '$VALUES_VER' but Chart.yaml bundles $BUNDLED"; FAILS=$((FAILS+1)); fi
expect "same version"                none "--set" "controller.clusterVersionOverride=$BUNDLED"
expect "same minor, other patch"     none "--set" "controller.clusterVersionOverride=$MINOR.99"
expect "v prefix"                    none "--set" "controller.clusterVersionOverride=v$BUNDLED"
expect "pre-release of same minor"   none "--set" "controller.clusterVersionOverride=v$MINOR.0-rc1"
expect "next minor"                  warn "--set" "controller.clusterVersionOverride=${MINOR%.*}.$(( ${MINOR#*.} + 1 )).0"
expect "previous minor"              warn "--set" "controller.clusterVersionOverride=${MINOR%.*}.$(( ${MINOR#*.} - 1 )).0"
expect "next major"                  warn "--set" "controller.clusterVersionOverride=$(( ${MINOR%.*} + 1 )).0.0"
expect "dev build (master-head)"     none "--set" "controller.clusterVersionOverride=master-head"
expect "garbage"                     none "--set" "controller.clusterVersionOverride=banana"
expect "version unknown (no lookup)" none
expect "controller disabled"         none "--set" "controller.enabled=false" "--set" "controller.clusterVersionOverride=9.9.9"
echo "== Support matrix"
if grep -q "\`harvester-vm-import-controller\` $BUNDLED" "$CHART/../../docs/support-matrix.md"; then echo "  PASS  docs/support-matrix.md lists the bundled controller $BUNDLED"
else echo "  FAIL  docs/support-matrix.md does not list the bundled controller $BUNDLED; update it"; FAILS=$((FAILS+1)); fi
echo; [ "$FAILS" = 0 ] && echo "passed" || { echo "$FAILS failed"; exit 1; }
